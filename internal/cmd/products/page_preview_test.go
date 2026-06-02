package products

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

func TestPagePreviewPostsHTMLToPreviewEndpoint(t *testing.T) {
	htmlPath := writePageHTML(t, "<script src=\"https://evil.test/x.js\"></script><h1>Buy</h1>")

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"custom_html": "<h1>Buy</h1>",
			"sanitization_report": map[string]any{
				"removed_tags": []map[string]any{{
					"tag":    "\x1b[31mscript\x1b[0m",
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
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotPath != "/products/prod1/preview_custom_html" {
		t.Errorf("got path %q, want /products/prod1/preview_custom_html", gotPath)
	}
	if got := gotForm.Get("custom_html"); got != "<script src=\"https://evil.test/x.js\"></script><h1>Buy</h1>" {
		t.Errorf("got custom_html=%q", got)
	}
	if !strings.Contains(out, "Previewed page") || !strings.Contains(out, "Sanitization removed 1 item") {
		t.Fatalf("output missing preview summary: %q", out)
	}
	if !strings.Contains(out, "script src host not allowed") {
		t.Fatalf("output missing report reason: %q", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("output should strip report ANSI controls: %q", out)
	}
}

func TestPagePreviewDryRunDoesNotCallAPI(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Buy</h1>")

	var calls atomic.Int32
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("preview --dry-run should not call API")
	})

	cmd := testutil.Command(newPagePreviewCmd(), testutil.DryRun(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if calls.Load() != 0 {
		t.Fatalf("API was called %d times", calls.Load())
	}
	if !strings.Contains(out, "Dry run: POST /products/prod1/preview_custom_html") {
		t.Fatalf("dry-run output missing preview request: %q", out)
	}
}

func TestPagePreviewSuccessFalseReturnsMessage(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Buy</h1>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success":false,"message":"Custom html is too long (maximum is 500000 characters)"}`)
	})

	cmd := testutil.Command(newPagePreviewCmd())
	cmd.SetArgs([]string{"prod1", htmlPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Custom html is too long") {
		t.Fatalf("expected validation message, got %v", err)
	}
}

func writePageHTML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "landing.html")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
