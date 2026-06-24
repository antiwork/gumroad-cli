package user

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUserPageURLPrintsProfileAndEmbedURLs(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"custom_html":      "<h1>Hi</h1>",
			"has_landing_page": true,
			"profile_url":      "https://jane.gumroad.com",
		})
	})

	cmd := testutil.Command(newPageURLCmd())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet {
		t.Errorf("got method %q, want GET", gotMethod)
	}
	if gotPath != "/user/custom_html" {
		t.Errorf("got path %q, want /user/custom_html", gotPath)
	}
	if !strings.Contains(out, "https://jane.gumroad.com") {
		t.Fatalf("output missing profile URL: %q", out)
	}
	if !strings.Contains(out, "https://jane.gumroad.com/landing/embed") {
		t.Fatalf("output missing embed URL: %q", out)
	}
}

func TestUserPageURLPlainPrintsBothColumns(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"has_landing_page": true,
			"profile_url":      "https://jane.gumroad.com",
		})
	})

	cmd := testutil.Command(newPageURLCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "https://jane.gumroad.com\thttps://jane.gumroad.com/landing/embed") {
		t.Fatalf("plain output should be tab-separated URLs: %q", out)
	}
}

func TestUserPageURLErrorsWhenProfileURLMissing(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"has_landing_page": false,
			"profile_url":      "",
		})
	})

	cmd := testutil.Command(newPageURLCmd())
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "did not include profile_url") {
		t.Fatalf("expected missing profile_url error, got %v", err)
	}
}

func TestUserCommandIncludesPageNamespace(t *testing.T) {
	cmd := NewUserCmd()
	found, _, err := cmd.Find([]string{"page", "publish"})
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if found == nil || found.Name() != "publish" {
		t.Fatalf("expected user page publish command, got %#v", found)
	}
}
