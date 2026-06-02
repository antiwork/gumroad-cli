package products

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageClearConfirmsAndSendsEmptyCustomHTML(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	var hasCustomHTML bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, hasCustomHTML = r.PostForm["custom_html"]
		previous := "<h1>Old</h1>"
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":          "prod1",
				"custom_html": "",
				"landing_url": "https://seller.gumroad.com/l/prod1",
			},
			"previous_custom_html": previous,
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/prod1" {
		t.Errorf("got path %q, want /products/prod1", gotPath)
	}
	if !hasCustomHTML {
		t.Fatalf("custom_html should be sent to clear")
	}
	if got := gotForm.Get("custom_html"); got != "" {
		t.Fatalf("got custom_html=%q, want empty", got)
	}
	if !strings.Contains(out, "Page cleared.") {
		t.Fatalf("output missing clear message: %q", out)
	}
}

func TestPageClearNoInputRequiresYesBeforeAPI(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("clear without confirmation should not call API")
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.NoInput(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"prod1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Use --yes to skip confirmation") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestPageClearRateLimitMessage(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		testutil.RawJSON(t, w, `{"success":false,"message":"Rate limited"}`)
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "Wait a moment before trying again") {
		t.Fatalf("expected clear-specific rate limit message, got %v", err)
	}
	if strings.Contains(err.Error(), "page preview") {
		t.Fatalf("clear rate limit message should not mention preview, got %v", err)
	}
}
