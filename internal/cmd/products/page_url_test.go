package products

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageURLPrintsLandingURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/products/prod1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"landing_url": "https://creator.example/l/prod1"},
		})
	})

	cmd := testutil.Command(newPageURLCmd())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if out != "https://creator.example/l/prod1\n" {
		t.Fatalf("out = %q", out)
	}
}

func TestPageURLJSONPreservesRawProduct(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success":true,"product":{"landing_url":"https://creator.example/l/prod1","custom_html":"<main>Live</main>"}}`)
	})

	cmd := testutil.Command(newPageURLCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, `"custom_html": "<main>Live</main>"`) {
		t.Fatalf("raw URL JSON not preserved: %q", out)
	}
}
