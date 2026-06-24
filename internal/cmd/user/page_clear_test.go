package user

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUserPageClearConfirmsAndSendsEmptyCustomHTML(t *testing.T) {
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
			"custom_html":          "",
			"previous_custom_html": previous,
			"profile_url":          "https://jane.gumroad.com",
			"sanitization_report":  emptyReport(),
		})
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/user/custom_html" {
		t.Errorf("got path %q, want /user/custom_html", gotPath)
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

func TestUserPageClearNoInputRequiresYesBeforeAPI(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("clear without confirmation should not call API")
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.NoInput(true), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Use --yes to skip confirmation") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestUserPageClearRateLimitMessage(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		testutil.RawJSON(t, w, `{"success":false,"message":"Rate limited"}`)
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Wait a moment before trying again") {
		t.Fatalf("expected clear-specific rate limit message, got %v", err)
	}
}
