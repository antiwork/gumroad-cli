package products

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPagePreviewSurfacesSuccessFalse(t *testing.T) {
	htmlPath := writePageHTML(t, strings.Repeat("x", 10))
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/products/prod1/preview_custom_html" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.RawJSON(t, w, `{"success":false,"message":"Custom html is too long (maximum is 500000 characters)"}`)
	})

	cmd := testutil.Command(newPagePreviewCmd())
	cmd.SetArgs([]string{"prod1", htmlPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "Custom html is too long") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPagePreviewJSONPreservesRawResponse(t *testing.T) {
	htmlPath := writePageHTML(t, "<main>Preview</main>")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success":true,"custom_html":"<main>Preview</main>","sanitization_report":{"removed_tags":[],"removed_attributes":[],"total_removed":0,"truncated":false}}`)
	})

	cmd := testutil.Command(newPagePreviewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, `"custom_html": "<main>Preview</main>"`) {
		t.Fatalf("raw preview JSON not preserved: %q", out)
	}
}
