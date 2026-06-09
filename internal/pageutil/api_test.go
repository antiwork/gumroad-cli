package pageutil

import (
	"errors"
	"net/http"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
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
