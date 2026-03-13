package cmdutil_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRunRequest_RendersResponse(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"email": "test@example.com"},
		})
	})

	var gotEmail string
	err := cmdutil.RunRequest(testutil.TestOptions(), "Fetching user...", "GET", "/user", url.Values{}, func(data json.RawMessage) error {
		var resp struct {
			User struct {
				Email string `json:"email"`
			} `json:"user"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		gotEmail = resp.User.Email
		return nil
	})
	if err != nil {
		t.Fatalf("RunRequest failed: %v", err)
	}
	if gotPath != "/user" {
		t.Fatalf("got path %q, want /user", gotPath)
	}
	if gotEmail != "test@example.com" {
		t.Fatalf("got email %q, want test@example.com", gotEmail)
	}
}

func TestRunRequest_JSONOutputSkipsRender(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"email": "json@example.com"},
		})
	})
	opts := testutil.TestOptions()
	opts.JSONOutput = true

	rendered := false
	out := testutil.CaptureStdout(func() {
		err := cmdutil.RunRequest(opts, "Fetching user...", "GET", "/user", url.Values{}, func(data json.RawMessage) error {
			rendered = true
			return nil
		})
		if err != nil {
			t.Fatalf("RunRequest failed: %v", err)
		}
	})

	if rendered {
		t.Fatal("render callback should not run in JSON mode")
	}
	if !strings.Contains(out, `"json@example.com"`) {
		t.Fatalf("expected JSON output, got %q", out)
	}
}

func TestRunRequest_JSONOutputSkipsRenderForWrite(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{"id": "sale_123"},
		})
	})
	opts := testutil.TestOptions()
	opts.JSONOutput = true

	rendered := false
	out := testutil.CaptureStdout(func() {
		err := cmdutil.RunRequest(opts, "Refunding sale...", "PUT", "/sales/sale_123/refund", url.Values{}, func(data json.RawMessage) error {
			rendered = true
			return nil
		})
		if err != nil {
			t.Fatalf("RunRequest failed: %v", err)
		}
	})

	if rendered {
		t.Fatal("render callback should not run in JSON mode")
	}
	if !strings.Contains(out, `"sale_123"`) {
		t.Fatalf("expected JSON output, got %q", out)
	}
}

func TestRunRequest_JSONOutputNormalizesEmptyBody(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := testutil.TestOptions()
	opts.JSONOutput = true

	out := testutil.CaptureStdout(func() {
		err := cmdutil.RunRequest(opts, "Deleting sale...", "DELETE", "/sales/sale_123", url.Values{}, func(data json.RawMessage) error {
			t.Fatal("render callback should not run in JSON mode")
			return nil
		})
		if err != nil {
			t.Fatalf("RunRequest failed: %v", err)
		}
	})

	if strings.TrimSpace(out) != "null" {
		t.Fatalf("expected null for empty JSON body, got %q", out)
	}
}

func TestRunRequestDecoded_RendersTypedResponse(t *testing.T) {
	type userResponse struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
	}

	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"email": "typed@example.com"},
		})
	})

	var gotEmail string
	err := cmdutil.RunRequestDecoded[userResponse](testutil.TestOptions(), "Fetching user...", "GET", "/user", url.Values{}, func(resp userResponse) error {
		gotEmail = resp.User.Email
		return nil
	})
	if err != nil {
		t.Fatalf("RunRequestDecoded failed: %v", err)
	}
	if gotPath != "/user" {
		t.Fatalf("got path %q, want /user", gotPath)
	}
	if gotEmail != "typed@example.com" {
		t.Fatalf("got email %q, want typed@example.com", gotEmail)
	}
}

func TestDecodeJSON_WrapsParseErrors(t *testing.T) {
	type payload struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
	}

	_, err := cmdutil.DecodeJSON[payload](json.RawMessage(`{"user":`))
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected wrapped parse error, got %v", err)
	}
}
