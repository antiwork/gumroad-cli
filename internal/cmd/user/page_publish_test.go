package user

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUserPagePublishPutsHTMLAndPrintsProfileURL(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<h1>Welcome</h1>")

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"custom_html":          "<h1>Welcome</h1>",
			"previous_custom_html": nil,
			"profile_url":          "https://jane.gumroad.com",
			"sanitization_report":  emptyReport(),
		})
	})

	cmd := testutil.Command(newPagePublishCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/user/custom_html" {
		t.Errorf("got path %q, want /user/custom_html", gotPath)
	}
	if got := gotForm.Get("custom_html"); got != "<h1>Welcome</h1>" {
		t.Errorf("got custom_html=%q", got)
	}
	if !strings.Contains(out, "Published page") || !strings.Contains(out, "Live at https://jane.gumroad.com") {
		t.Fatalf("output missing publish summary: %q", out)
	}
}

func TestUserPagePublishReadsStdin(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"custom_html":          "<h1>From stdin</h1>",
			"previous_custom_html": nil,
			"profile_url":          "https://jane.gumroad.com",
			"sanitization_report":  emptyReport(),
		})
	})

	cmd := testutil.Command(newPagePublishCmd(),
		testutil.Quiet(false), testutil.NoColor(true),
		testutil.Stdin(strings.NewReader("<h1>From stdin</h1>")))
	cmd.SetArgs([]string{"-"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := gotForm.Get("custom_html"); got != "<h1>From stdin</h1>" {
		t.Errorf("got custom_html=%q from stdin", got)
	}
}

func TestUserPagePublishJSONPrintsRawResponse(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<h1>Welcome</h1>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"custom_html":          "<h1>Welcome</h1>",
			"previous_custom_html": nil,
			"profile_url":          "https://jane.gumroad.com",
			"sanitization_report":  emptyReport(),
		})
	})

	cmd := testutil.Command(newPagePublishCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if resp["profile_url"] != "https://jane.gumroad.com" {
		t.Fatalf("JSON output missing profile_url: %s", out)
	}
}

func TestUserPagePublishDryRunDoesNotCallAPI(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<h1>Welcome</h1>")

	var calls atomic.Int32
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("publish --dry-run should not call API")
	})

	cmd := testutil.Command(newPagePublishCmd(), testutil.DryRun(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if calls.Load() != 0 {
		t.Fatalf("API was called %d times", calls.Load())
	}
	if !strings.Contains(out, "Dry run: PUT /user/custom_html") {
		t.Fatalf("dry-run output missing publish request: %q", out)
	}
}

func TestUserPagePublishRateLimitMessage(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<h1>Welcome</h1>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		testutil.RawJSON(t, w, `{"success":false,"message":"Rate limited"}`)
	})

	cmd := testutil.Command(newPagePublishCmd())
	cmd.SetArgs([]string{htmlPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "30 PUTs/min") {
		t.Fatalf("expected publish-specific rate limit message, got %v", err)
	}
	if !strings.Contains(err.Error(), "user page preview") {
		t.Fatalf("publish rate limit should point at user page preview, got %v", err)
	}
}
