package user

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUserPagePreviewPostsHTMLToPreviewEndpoint(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<script src=\"https://evil.test/x.js\"></script><h1>Hi</h1>")

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"custom_html": "<h1>Hi</h1>",
			"sanitization_report": map[string]any{
				"removed_tags": []map[string]any{{
					"tag":    "script",
					"attrs":  map[string]string{"src": "https://evil.test/x.js"},
					"reason": "script src host not allowed",
				}},
				"removed_attributes": []any{},
				"total_removed":      1,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePreviewCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotPath != "/user/preview_custom_html" {
		t.Errorf("got path %q, want /user/preview_custom_html", gotPath)
	}
	if got := gotForm.Get("custom_html"); got != "<script src=\"https://evil.test/x.js\"></script><h1>Hi</h1>" {
		t.Errorf("got custom_html=%q", got)
	}
	if !strings.Contains(out, "Previewed page") || !strings.Contains(out, "Sanitization removed 1 item") {
		t.Fatalf("output missing preview summary: %q", out)
	}
	if !strings.Contains(out, "script src host not allowed") {
		t.Fatalf("output missing report reason: %q", out)
	}
}

func TestUserPagePreviewDefaultsToLandingHTML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "landing.html"), []byte("<h1>Hi</h1>"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Chdir(dir)

	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"custom_html":         "<h1>Hi</h1>",
			"sanitization_report": emptyReport(),
		})
	})

	cmd := testutil.Command(newPagePreviewCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := gotForm.Get("custom_html"); got != "<h1>Hi</h1>" {
		t.Errorf("got custom_html=%q from default ./landing.html", got)
	}
}

func TestUserPagePreviewDryRunDoesNotCallAPI(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<h1>Hi</h1>")

	var calls atomic.Int32
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("preview --dry-run should not call API")
	})

	cmd := testutil.Command(newPagePreviewCmd(), testutil.DryRun(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if calls.Load() != 0 {
		t.Fatalf("API was called %d times", calls.Load())
	}
	if !strings.Contains(out, "Dry run: POST /user/preview_custom_html") {
		t.Fatalf("dry-run output missing preview request: %q", out)
	}
}

func TestUserPagePreviewRateLimitMessage(t *testing.T) {
	htmlPath := writeProfilePageHTML(t, "<h1>Hi</h1>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		testutil.RawJSON(t, w, `{"success":false,"message":"Rate limited"}`)
	})

	cmd := testutil.Command(newPagePreviewCmd())
	cmd.SetArgs([]string{htmlPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "60 previews/min") {
		t.Fatalf("expected preview-specific rate limit message, got %v", err)
	}
}

func TestUserPagePreviewRejectsExtraArg(t *testing.T) {
	cmd := testutil.Command(newPagePreviewCmd())
	cmd.SetArgs([]string{"./landing.html", "unexpected"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("expected usage error for extra arg, got %v", err)
	}
}

func writeProfilePageHTML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "landing.html")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func emptyReport() map[string]any {
	return map[string]any{
		"removed_tags":       []any{},
		"removed_attributes": []any{},
		"total_removed":      0,
		"truncated":          false,
	}
}
