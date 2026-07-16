package pageutil

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/config"
)

func TestShareURLPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		product PageProduct
		want    string
	}{
		{"prefers short_url", PageProduct{ShortURL: "https://s/l/a", LandingURL: "https://s/l/b", PermalinkURL: "https://s/l/c"}, "https://s/l/a"},
		{"falls back to landing_url", PageProduct{LandingURL: "https://s/l/b", PermalinkURL: "https://s/l/c"}, "https://s/l/b"},
		{"falls back to permalink_url", PageProduct{PermalinkURL: "https://s/l/c"}, "https://s/l/c"},
		{"empty when none present", PageProduct{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShareURL(tc.product); got != tc.want {
				t.Fatalf("ShareURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTranslateRateLimitErrorPreservesAPIError(t *testing.T) {
	err := TranslateRateLimitError(&api.APIError{
		StatusCode: http.StatusTooManyRequests,
		Message:    "Rate limited.",
		Hint:       "Wait a moment and retry.",
	}, PublishRateLimitMessage)

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("translated error should preserve *api.APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("got status %d, want 429", apiErr.StatusCode)
	}
	if apiErr.Message != PublishRateLimitMessage {
		t.Fatalf("got message %q, want %q", apiErr.Message, PublishRateLimitMessage)
	}
	if apiErr.Hint != "Wait a moment and retry." {
		t.Fatalf("got hint %q", apiErr.Hint)
	}
}

func TestTranslateMissingScopeErrorRewritesEditProfile403(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")

	err := TranslateMissingScopeError(&api.APIError{
		StatusCode: http.StatusForbidden,
		Message:    "Access denied: This endpoint requires the edit_profile scope.",
		Hint:       "Check that your token has the required scope.",
	})

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("translated error should preserve *api.APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("got status %d, want 403", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "edit_profile scope") {
		t.Fatalf("message should name the missing scope, got %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Hint, "gumroad auth login") {
		t.Fatalf("hint should tell the user to re-authenticate, got %q", apiErr.Hint)
	}
	if strings.Contains(apiErr.Hint, config.EnvAccessToken) {
		t.Fatalf("config-sourced token should not get the env-var hint, got %q", apiErr.Hint)
	}
}

func TestTranslateMissingScopeErrorPointsAtEnvToken(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "old-account-only-token")

	err := TranslateMissingScopeError(&api.APIError{
		StatusCode: http.StatusForbidden,
		Message:    "Access denied: This endpoint requires the edit_profile scope.",
	})

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("translated error should preserve *api.APIError, got %T", err)
	}
	if !strings.Contains(apiErr.Hint, config.EnvAccessToken) {
		t.Fatalf("hint should name the environment variable, got %q", apiErr.Hint)
	}
	if !strings.Contains(apiErr.Hint, "overrides any saved login") {
		t.Fatalf("hint should explain the env token takes precedence over login, got %q", apiErr.Hint)
	}
	if !strings.Contains(apiErr.Hint, "unset it and run: gumroad auth login") {
		t.Fatalf("hint should offer the unset-and-relogin path, got %q", apiErr.Hint)
	}
}

func TestTranslateMissingScopeErrorLeavesOtherErrorsAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"unrelated 403", &api.APIError{StatusCode: http.StatusForbidden, Message: "Access denied."}},
		{"401", &api.APIError{StatusCode: http.StatusUnauthorized, Message: "Not authenticated."}},
		{"non-API error", errors.New("network down")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateMissingScopeError(tc.err)
			if !errors.Is(got, tc.err) {
				t.Fatalf("error should pass through unchanged, got %v", got)
			}
			var wantAPIErr *api.APIError
			if errors.As(tc.err, &wantAPIErr) {
				var gotAPIErr *api.APIError
				if !errors.As(got, &gotAPIErr) || gotAPIErr.Message != wantAPIErr.Message {
					t.Fatalf("message should be untouched, got %v", got)
				}
			}
		})
	}
}
