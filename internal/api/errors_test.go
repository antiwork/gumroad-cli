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
	if apiErr.Message != "Not authenticated." {
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
	if apiErr.Message != "Access denied." {
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

func TestAPIError_Hints(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantHint   string
	}{
		{"401", 401, `{"success":false}`, HintRunAuthLogin},
		{"403", 403, `{"success":false}`, HintMissingScope},
		{"403 with an endpoint message", 403, `{"success":false,"message":"Custom HTML pages are not enabled for your account."}`, "Check that your token has the required scope."},
		{"404", 404, `{"success":false}`, "Check the resource ID and try again."},
		{"429", 429, `{"success":false,"message":"Rate limited"}`, HintRateLimited},
		{"429 without a message", 429, `{"success":false}`, HintRateLimited},
		{"500", 500, `{"success":false}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := mustAPIError(t, parseAPIError(tt.statusCode, []byte(tt.body)))
			if apiErr.Hint != tt.wantHint {
				t.Errorf("got hint %q, want %q", apiErr.Hint, tt.wantHint)
			}
		})
	}
}

func TestHintedErrorInterface(t *testing.T) {
	err := &APIError{StatusCode: 401, Message: "Not authenticated.", Hint: HintRunAuthLogin}
	var hinted HintedError
	if !errors.As(err, &hinted) {
		t.Fatal("expected APIError to satisfy HintedError")
	}
	if hinted.GetHint() != HintRunAuthLogin {
		t.Errorf("got hint %q", hinted.GetHint())
	}
}

func TestAPIError_GetHint_Nil(t *testing.T) {
	var err *APIError
	if err.GetHint() != "" {
		t.Errorf("expected empty hint from nil receiver")
	}
}

func TestParseAPIError_StructuredErrorCode(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantMessage string
		wantCode    string
	}{
		{
			name:        "NoSuchUpload structured response",
			status:      400,
			body:        `{"status":400,"error":"The specified upload does not exist.","error_code":"multipart_upload_not_found"}`,
			wantMessage: "The specified upload does not exist.",
			wantCode:    "multipart_upload_not_found",
		},
		{
			name:        "legacy response without code",
			status:      400,
			body:        `{"status":400,"error":"The specified upload does not exist."}`,
			wantMessage: "The specified upload does not exist.",
			wantCode:    "",
		},
		{
			name:        "unrelated structured code",
			status:      400,
			body:        `{"error":"invalid key","error_code":"unrelated_error"}`,
			wantMessage: "invalid key",
			wantCode:    "unrelated_error",
		},
		{
			name:        "code without message",
			status:      400,
			body:        `{"error_code":"multipart_upload_not_found"}`,
			wantMessage: "API error (HTTP 400)",
			wantCode:    "multipart_upload_not_found",
		},
		{
			name:        "message takes precedence over error",
			status:      400,
			body:        `{"message":"preferred","error":"fallback","error_code":"multipart_upload_not_found"}`,
			wantMessage: "preferred",
			wantCode:    "multipart_upload_not_found",
		},
		{
			name:        "numeric code preserves legacy error",
			status:      400,
			body:        `{"error":"invalid key","error_code":42}`,
			wantMessage: "invalid key",
			wantCode:    "",
		},
		{
			name:        "object code preserves legacy error",
			status:      400,
			body:        `{"error":"invalid key","error_code":{"unexpected":true}}`,
			wantMessage: "invalid key",
			wantCode:    "",
		},
		{
			name:        "null code preserves legacy error",
			status:      400,
			body:        `{"error":"invalid key","error_code":null}`,
			wantMessage: "invalid key",
			wantCode:    "",
		},
		{
			name:        "malformed JSON cannot supply a code",
			status:      400,
			body:        `{"error":"invalid key","error_code":"multipart_upload_not_found"`,
			wantMessage: "API error (HTTP 400)",
			wantCode:    "",
		},
		{
			name:        "code parsing does not change HTTP status",
			status:      500,
			body:        `{"error":"S3 unavailable","error_code":"multipart_upload_not_found"}`,
			wantMessage: "S3 unavailable",
			wantCode:    "multipart_upload_not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := mustAPIError(t, parseAPIError(tt.status, []byte(tt.body)))

			if apiErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
			}
			if apiErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", apiErr.Message, tt.wantMessage)
			}
			if apiErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", apiErr.Code, tt.wantCode)
			}
		})
	}
}
