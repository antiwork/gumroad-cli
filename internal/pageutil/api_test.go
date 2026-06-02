package pageutil

import (
	"errors"
	"net/http"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
)

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
