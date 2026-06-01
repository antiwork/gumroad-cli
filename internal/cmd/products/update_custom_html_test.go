package products

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUpdate_CustomHTMLFromFile(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "landing.html")
	body := "<section><h1>Buy now</h1></section>"
	if err := os.WriteFile(htmlPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--custom-html", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/prod1" {
		t.Errorf("got path %q, want /products/prod1", gotPath)
	}
	if got := gotForm.Get("custom_html"); got != body {
		t.Errorf("got custom_html=%q, want %q", got, body)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("expected updated message, got: %q", out)
	}
}

func TestUpdate_CustomHTMLEmptyClears(t *testing.T) {
	var gotForm url.Values
	var hasField bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, hasField = r.PostForm["custom_html"]
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--custom-html", ""})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !hasField {
		t.Errorf("custom_html should be sent (empty) to clear, but was absent")
	}
	if got := gotForm.Get("custom_html"); got != "" {
		t.Errorf("got custom_html=%q, want empty string", got)
	}
}

func TestUpdate_CustomHTMLMissingFileErrors(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API on a bad --custom-html path")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"prod1", "--custom-html", filepath.Join(t.TempDir(), "does-not-exist.html")})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--custom-html") {
		t.Errorf("expected --custom-html read error, got: %v", err)
	}
}
