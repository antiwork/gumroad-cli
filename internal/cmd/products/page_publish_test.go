package products

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPagePublishPutsHTMLAndPrintsLandingURL(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Buy</h1>")

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":          "prod1",
				"custom_html": "<h1>Buy</h1>",
				"landing_url": "https://seller.gumroad.com/l/prod1",
			},
			"previous_custom_html": nil,
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePublishCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/prod1" {
		t.Errorf("got path %q, want /products/prod1", gotPath)
	}
	if got := gotForm.Get("custom_html"); got != "<h1>Buy</h1>" {
		t.Errorf("got custom_html=%q", got)
	}
	if !strings.Contains(out, "Published page") || !strings.Contains(out, "Live at https://seller.gumroad.com/l/prod1") {
		t.Fatalf("output missing publish summary: %q", out)
	}
}

func TestPagePublishJSONPrintsRawResponse(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Buy</h1>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":          "prod1",
				"custom_html": "<h1>Buy</h1>",
				"landing_url": "https://seller.gumroad.com/l/prod1",
			},
			"previous_custom_html": nil,
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePublishCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if _, ok := resp["result"]; ok {
		t.Fatalf("page publish JSON should be raw API response, got mutation wrapper: %s", out)
	}
	if product, ok := resp["product"].(map[string]any); !ok || product["landing_url"] != "https://seller.gumroad.com/l/prod1" {
		t.Fatalf("JSON output missing product landing_url: %s", out)
	}
}

func TestPagePublishDryRunDoesNotCallAPI(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Buy</h1>")

	var calls atomic.Int32
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("publish --dry-run should not call API")
	})

	cmd := testutil.Command(newPagePublishCmd(), testutil.DryRun(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if calls.Load() != 0 {
		t.Fatalf("API was called %d times", calls.Load())
	}
	if !strings.Contains(out, "Dry run: PUT /products/prod1") {
		t.Fatalf("dry-run output missing publish request: %q", out)
	}
}

func TestPagePublishRateLimitMessage(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Buy</h1>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		testutil.RawJSON(t, w, `{"success":false,"message":"Rate limited"}`)
	})

	cmd := testutil.Command(newPagePublishCmd())
	cmd.SetArgs([]string{"prod1", htmlPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "30 PUTs/min") {
		t.Fatalf("expected page-specific rate limit message, got %v", err)
	}
}
