package pageutil

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
)

const (
	PublishRateLimitMessage = "Hit Gumroad's rate limit (30 PUTs/min). Use `gumroad products page preview` to iterate without burning your publish budget."
	PreviewRateLimitMessage = "Hit Gumroad's rate limit (60 previews/min). Wait a moment before previewing again."
	ClearRateLimitMessage   = "Hit Gumroad's rate limit (30 PUTs/min). Wait a moment before trying again."

	ProfilePublishRateLimitMessage = "Hit Gumroad's rate limit (30 PUTs/min). Use `gumroad user page preview` to iterate without burning your publish budget."
	ProfilePreviewRateLimitMessage = "Hit Gumroad's rate limit (60 previews/min). Wait a moment before previewing again."
	ProfileClearRateLimitMessage   = "Hit Gumroad's rate limit (30 PUTs/min). Wait a moment before trying again."

	PagesPublishRateLimitMessage = "Hit Gumroad's rate limit (30 PUTs/min). Use `gumroad pages preview` to iterate without burning your publish budget."
	PagesPreviewRateLimitMessage = "Hit Gumroad's rate limit (60 previews/min). Wait a moment before previewing again."
)

type Target struct {
	Path        string
	PreviewPath string
}

func ProductTarget(id string) Target {
	path := cmdutil.JoinPath("products", id)
	return Target{
		Path:        path,
		PreviewPath: cmdutil.JoinPath("products", id, "preview_custom_html"),
	}
}

type PageProduct struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CustomHTML   string `json:"custom_html"`
	LandingURL   string `json:"landing_url"`
	ShortURL     string `json:"short_url"`
	PermalinkURL string `json:"permalink_url"`
}

type UpdateResponse struct {
	Success            bool               `json:"success"`
	Product            PageProduct        `json:"product"`
	PreviousCustomHTML *string            `json:"previous_custom_html"`
	SanitizationReport SanitizationReport `json:"sanitization_report"`
}

type PreviewResponse struct {
	Success            bool               `json:"success"`
	CustomHTML         string             `json:"custom_html"`
	SanitizationReport SanitizationReport `json:"sanitization_report"`
}

type ShowResponse struct {
	Success bool        `json:"success"`
	Product PageProduct `json:"product"`
}

func LandingURL(product PageProduct) string {
	if product.LandingURL != "" {
		return product.LandingURL
	}
	if product.ShortURL != "" {
		return product.ShortURL
	}
	return product.PermalinkURL
}

// ShareURL returns the product's public share link, preferring the canonical
// short_url and falling back to landing_url then permalink_url.
func ShareURL(product PageProduct) string {
	if product.ShortURL != "" {
		return product.ShortURL
	}
	if product.LandingURL != "" {
		return product.LandingURL
	}
	return product.PermalinkURL
}

// ProfileTarget points at the seller's own profile landing page. Unlike a
// product there is no id — the API derives the seller from the access token.
func ProfileTarget() Target {
	return Target{
		Path:        cmdutil.JoinPath("user", "custom_html"),
		PreviewPath: cmdutil.JoinPath("user", "preview_custom_html"),
	}
}

type ProfileUpdateResponse struct {
	Success            bool               `json:"success"`
	CustomHTML         string             `json:"custom_html"`
	PreviousCustomHTML *string            `json:"previous_custom_html"`
	ProfileURL         string             `json:"profile_url"`
	SanitizationReport SanitizationReport `json:"sanitization_report"`
}

type ProfileShowResponse struct {
	Success        bool   `json:"success"`
	CustomHTML     string `json:"custom_html"`
	HasLandingPage bool   `json:"has_landing_page"`
	ProfileURL     string `json:"profile_url"`
}

func ProfilePreviousHTML(resp ProfileUpdateResponse) string {
	if resp.PreviousCustomHTML == nil {
		return ""
	}
	return *resp.PreviousCustomHTML
}

// ProfileEmbedURL is where the sandboxed custom HTML renders inside the public
// profile — the profile URL with the landing/embed suffix the backend serves.
func ProfileEmbedURL(profileURL string) string {
	if profileURL == "" {
		return ""
	}
	return strings.TrimSuffix(profileURL, "/") + "/landing/embed"
}

func HTMLParams(html string) url.Values {
	return url.Values{"custom_html": []string{html}}
}

func ClearParams() url.Values {
	return url.Values{"custom_html": []string{""}}
}

func PreviousHTML(resp UpdateResponse) string {
	if resp.PreviousCustomHTML == nil {
		return ""
	}
	return *resp.PreviousCustomHTML
}

func TranslateRateLimitError(err error, message string) error {
	if err == nil {
		return nil
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    message,
			Hint:       apiErr.Hint,
		}
	}
	return err
}

// TranslateMissingScopeError rewrites the pages endpoints' scope rejection into
// a re-authentication hint. Tokens minted before the CLI requested the
// edit_profile scope can never pass the pages write gate — without this hint
// the raw scope error reads like an account or product problem. The recovery
// depends on where the token came from: `gumroad auth login` only rewrites the
// config file, so when GUMROAD_ACCESS_TOKEN is set (it takes precedence over
// the saved login) the user has to fix the environment variable instead.
func TranslateMissingScopeError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden && strings.Contains(apiErr.Message, "requires the edit_profile scope") {
		hint := "Run: gumroad auth login to re-authenticate with the updated scopes."
		// Match config.ResolveToken's normalization: a whitespace-only
		// environment value does not override the saved login, so it must
		// not flip the hint either.
		if strings.TrimSpace(os.Getenv(config.EnvAccessToken)) != "" {
			hint = "Your token comes from " + config.EnvAccessToken + ", which overrides any saved login. Update it to a token that has the edit_profile scope, or unset it and run: gumroad auth login"
		}
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    "Your access token doesn't have the edit_profile scope, which writing storefront pages requires.",
			Hint:       hint,
		}
	}
	return err
}
