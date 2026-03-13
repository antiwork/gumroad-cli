package cmdutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
)

type testUserResponse struct {
	User struct {
		Email string `json:"email"`
	} `json:"user"`
}

func TestPrintDryRunAction_DefaultOutput(t *testing.T) {
	setColorEnabledForTest(t, false)

	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintDryRunAction(opts, "remove stored API token"); err != nil {
		t.Fatalf("PrintDryRunAction returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Dry run: remove stored API token") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestFetchPage_NilClient(t *testing.T) {
	if _, err := FetchPage[testPage](nil, "/items", nil); err == nil || !strings.Contains(err.Error(), "nil api client") {
		t.Fatalf("expected nil client error, got %v", err)
	}
}

func TestRunDecoded_RendersTypedResponse(t *testing.T) {
	t.Setenv("GUMROAD_ACCESS_TOKEN", "test-token")
	var rendered string
	err := RunDecoded[testUserResponse](DefaultOptions(), "", func(*api.Client) (json.RawMessage, error) {
		return json.RawMessage(`{"user":{"email":"decoded@example.com"}}`), nil
	}, func(resp testUserResponse) error {
		rendered = resp.User.Email
		return nil
	})
	if err != nil {
		t.Fatalf("RunDecoded failed: %v", err)
	}
	if rendered != "decoded@example.com" {
		t.Fatalf("got rendered email %q", rendered)
	}
}

func TestRunDecoded_JSONOutputSkipsTypedRender(t *testing.T) {
	t.Setenv("GUMROAD_ACCESS_TOKEN", "test-token")
	opts := DefaultOptions()
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := RunDecoded[testUserResponse](opts, "", func(*api.Client) (json.RawMessage, error) {
		return json.RawMessage(`{"user":{"email":"json-decoded@example.com"}}`), nil
	}, func(testUserResponse) error {
		t.Fatal("typed render should not run in JSON mode")
		return nil
	})
	if err != nil {
		t.Fatalf("RunDecoded failed: %v", err)
	}
	if !strings.Contains(out.String(), `"json-decoded@example.com"`) {
		t.Fatalf("unexpected JSON output: %q", out.String())
	}
}

func TestRunDecoded_InvalidJSONReturnsParseError(t *testing.T) {
	t.Setenv("GUMROAD_ACCESS_TOKEN", "test-token")
	err := RunDecoded[testPage](DefaultOptions(), "", func(*api.Client) (json.RawMessage, error) {
		return json.RawMessage(`not-json`), nil
	}, func(testPage) error {
		t.Fatal("render should not run for invalid JSON")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWithToken_JSONOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := RunWithToken(opts, "tok", "", func(*api.Client) (json.RawMessage, error) {
		return json.RawMessage(`{"user":{"email":"json@example.com"}}`), nil
	}, func(json.RawMessage) error {
		t.Fatal("render should not run in JSON mode")
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithToken failed: %v", err)
	}
	if !strings.Contains(out.String(), `"json@example.com"`) {
		t.Fatalf("unexpected JSON output: %q", out.String())
	}
}

func TestRunWithToken_RendersInHumanMode(t *testing.T) {
	var rendered string
	err := RunWithToken(DefaultOptions(), "tok", "", func(*api.Client) (json.RawMessage, error) {
		return json.RawMessage(`{"user":{"email":"human@example.com"}}`), nil
	}, func(data json.RawMessage) error {
		rendered = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithToken failed: %v", err)
	}
	if !strings.Contains(rendered, "human@example.com") {
		t.Fatalf("unexpected render payload: %q", rendered)
	}
}

func TestRunRequestDecoded_DryRunSkipsRender(t *testing.T) {
	opts := DefaultOptions()
	opts.DryRun = true
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := RunRequestDecoded[testPage](opts, "", "PUT", "/items", url.Values{"page_key": {"page-2"}}, func(testPage) error {
		t.Fatal("render should not run in dry-run mode")
		return nil
	})
	if err != nil {
		t.Fatalf("RunRequestDecoded failed: %v", err)
	}
	if !strings.Contains(out.String(), "PUT\t/items\tpage_key=page-2") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}
}

func TestRunAuthenticatedData_UsesStoredToken(t *testing.T) {
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		writeSuccessJSON(t, w, map[string]any{"user": map[string]any{"email": "demo@example.com"}})
	})

	opts := DefaultOptions()
	opts.Quiet = true
	opts.Version = "test"

	data, err := runAuthenticatedData(opts, "Fetching...", func(client *api.Client) (json.RawMessage, error) {
		return client.Get("/user", nil)
	})
	if err != nil {
		t.Fatalf("runAuthenticatedData failed: %v", err)
	}
	if !strings.Contains(string(data), "demo@example.com") {
		t.Fatalf("unexpected response: %q", data)
	}
}

func TestRunAuthenticatedData_MissingTokenReturnsError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GUMROAD_ACCESS_TOKEN", "")

	_, err := runAuthenticatedData(DefaultOptions(), "Fetching...", func(*api.Client) (json.RawMessage, error) {
		t.Fatal("request should not run without auth")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintDryRunJSON_MarshalError(t *testing.T) {
	err := printDryRunJSON(DefaultOptions(), map[string]any{"bad": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "could not encode dry-run output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancelledActionMessage_NormalizesInput(t *testing.T) {
	if got := cancelledActionMessage(""); got != cancelledLabel {
		t.Fatalf("got %q, want %q", got, cancelledLabel)
	}
	if got := cancelledActionMessage("delete sale."); got != "Cancelled: delete sale." {
		t.Fatalf("got %q, want %q", got, "Cancelled: delete sale.")
	}
}

func TestPrintCancelledAction_QuietNoOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.Quiet = true
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintCancelledAction(opts, "delete product prod_123"); err != nil {
		t.Fatalf("PrintCancelledAction returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no quiet output, got %q", out.String())
	}
}

func TestUsageHelperEdgeCases(t *testing.T) {
	if got := filteredExample("  gumroad products list\n\n  gumroad sales list", "gumroad users"); got != "" {
		t.Fatalf("expected no matching examples, got %q", got)
	}

	if got := generatedExample(nil); got != "" {
		t.Fatalf("expected empty example for nil command, got %q", got)
	}

	if isRequiredExampleFlag(nil) {
		t.Fatal("nil flag should not be treated as required")
	}
}

func TestNewPageKeyTracker_SeedsExistingPageKey(t *testing.T) {
	tracker := newPageKeyTracker(url.Values{"page_key": {"page-2"}})

	if err := tracker.Track("page-2"); err == nil || !strings.Contains(err.Error(), `pagination cycle detected for page_key "page-2"`) {
		t.Fatalf("unexpected repeated key error: %v", err)
	}
	if err := tracker.Track("page-3"); err != nil {
		t.Fatalf("unexpected new page key error: %v", err)
	}
}
