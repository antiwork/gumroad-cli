package api

import (
	"errors"
	"testing"
)

func mustAPIError(t *testing.T, err error) *APIError {
	t.Helper()

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	return apiErr
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 404, Message: "not found"}
	if err.Error() != "not found" {
		t.Errorf("got %q, want %q", err.Error(), "not found")
	}
}

func TestParseAPIError_WithMessage(t *testing.T) {
	body := []byte(`{"success":false,"message":"The product was not found"}`)
	apiErr := mustAPIError(t, parseAPIError(404, body))
	if apiErr.StatusCode != 404 {
		t.Errorf("got status %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "The product was not found" {
		t.Errorf("got message %q, want %q", apiErr.Message, "The product was not found")
	}
}

func TestParseAPIError_401(t *testing.T) {
	body := []byte(`{"success":false}`)
	err := parseAPIError(401, body)
	apiErr := mustAPIError(t, err)
	if apiErr.Message != "Not authenticated. Run `gumroad auth login` or set `GUMROAD_ACCESS_TOKEN`." {
		t.Errorf("got message %q", apiErr.Message)
	}
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatal("expected errors.Is(err, ErrNotAuthenticated)")
	}
}

func TestParseAPIError_403_WithMessage(t *testing.T) {
	body := []byte(`{"success":false,"message":"Insufficient scope"}`)
	err := parseAPIError(403, body)
	apiErr := mustAPIError(t, err)
	if apiErr.Message != "Access denied: Insufficient scope" {
		t.Errorf("got message %q", apiErr.Message)
	}
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatal("expected errors.Is(err, ErrAccessDenied)")
	}
}

func TestParseAPIError_403_NoMessage(t *testing.T) {
	body := []byte(`{"success":false}`)
	apiErr := mustAPIError(t, parseAPIError(403, body))
	if apiErr.Message != "Access denied. Your token may not have the required scope." {
		t.Errorf("got message %q", apiErr.Message)
	}
}

func TestParseAPIError_404_NoMessage(t *testing.T) {
	body := []byte(`{"success":false}`)
	err := parseAPIError(404, body)
	apiErr := mustAPIError(t, err)
	if apiErr.Message != "Resource not found." {
		t.Errorf("got message %q", apiErr.Message)
	}
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatal("expected errors.Is(err, ErrResourceNotFound)")
	}
}

func TestParseAPIError_DefaultWithMessage(t *testing.T) {
	body := []byte(`{"success":false,"message":"Rate limited"}`)
	apiErr := mustAPIError(t, parseAPIError(429, body))
	if apiErr.Message != "Rate limited" {
		t.Errorf("got message %q", apiErr.Message)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("got status %d, want 429", apiErr.StatusCode)
	}
}

func TestParseAPIError_DefaultNoMessage(t *testing.T) {
	body := []byte(`{"success":false}`)
	apiErr := mustAPIError(t, parseAPIError(502, body))
	if apiErr.Message != "API error (HTTP 502)" {
		t.Errorf("got message %q", apiErr.Message)
	}
}

func TestParseAPIError_InvalidJSON(t *testing.T) {
	body := []byte(`not json`)
	apiErr := mustAPIError(t, parseAPIError(500, body))
	if apiErr.StatusCode != 500 {
		t.Errorf("got status %d, want 500", apiErr.StatusCode)
	}
	if apiErr.Message != "API error (HTTP 500)" {
		t.Errorf("got message %q", apiErr.Message)
	}
}

func TestParseAPIError_EmptyJSON(t *testing.T) {
	body := []byte(`{}`)
	apiErr := mustAPIError(t, parseAPIError(500, body))
	// Empty message in JSON → falls through to default
	if apiErr.Message != "API error (HTTP 500)" {
		t.Errorf("got message %q", apiErr.Message)
	}
}

func TestAPIErrorIs(t *testing.T) {
	err401 := &APIError{StatusCode: 401, Message: "auth"}
	if !errors.Is(err401, ErrNotAuthenticated) {
		t.Fatal("expected 401 error to match ErrNotAuthenticated")
	}
	if errors.Is(err401, ErrAccessDenied) {
		t.Fatal("did not expect 401 error to match ErrAccessDenied")
	}

	err403 := &APIError{StatusCode: 403, Message: "access"}
	if !errors.Is(err403, ErrAccessDenied) {
		t.Fatal("expected 403 error to match ErrAccessDenied")
	}

	err404 := &APIError{StatusCode: 404, Message: "missing"}
	if !errors.Is(err404, ErrResourceNotFound) {
		t.Fatal("expected 404 error to match ErrResourceNotFound")
	}

	var nilAPIError *APIError
	if errors.Is(nilAPIError, ErrNotAuthenticated) {
		t.Fatal("did not expect nil APIError to match")
	}
	if err401.Is(nil) {
		t.Fatal("did not expect nil target to match")
	}
}
