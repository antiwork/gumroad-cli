package pageutil

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

const (
	PublishRateLimitMessage = "Hit Gumroad's rate limit (30 PUTs/min). Use `gumroad products page preview` to iterate without burning your publish budget."
	PreviewRateLimitMessage = "Hit Gumroad's rate limit (60 previews/min). Wait a moment before previewing again."
	ClearRateLimitMessage   = "Hit Gumroad's rate limit (30 PUTs/min). Wait a moment before trying again."
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
