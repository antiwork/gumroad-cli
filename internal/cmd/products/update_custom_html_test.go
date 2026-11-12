package products

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/antiwork/gumroad-cli/internal/pageutil"
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

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--custom-html", filepath.Join(t.TempDir(), "does-not-exist.html")})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--custom-html") {
		t.Errorf("expected --custom-html read error, got: %v", err)
	}
}

func TestUpdate_CustomHTMLRejectsAnOversizePageWithoutCallingTheAPI(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "landing.html")
	if err := os.WriteFile(htmlPath, []byte(strings.Repeat("a", pageutil.MaxCustomHTMLLength+1)), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var calls atomic.Int32
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("an oversize page should be rejected before the request, not sent")
	})

	cmd := testutil.Command(newUpdateCmd())
	cmd.SetArgs([]string{"prod1", "--custom-html", htmlPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want a local error for a page over the limit")
	}
	if calls.Load() != 0 {
		t.Fatalf("API was called %d times", calls.Load())
	}
	if !strings.Contains(err.Error(), "500000") {
		t.Fatalf("error should name the limit, got %q", err)
	}
}

func TestUpdate_CustomHTMLWithFileSkipsTheFormBodyLimitBecauseTheBodyIsJSON(t *testing.T) {
	const formBodyLimit = 4 << 20
	page := strings.Repeat("😀", 400_000)
	if form := (url.Values{"custom_html": {page}}).Encode(); len(form) <= formBodyLimit {
		t.Fatalf("fixture is %d bytes form-encoded, want over 4 MiB", len(form))
	}
	htmlPath := filepath.Join(t.TempDir(), "landing.html")
	if err := os.WriteFile(htmlPath, []byte(page), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	srv := newProductUpdateFileServers(t)
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1", "--custom-html", htmlPath, "--file", writeProductUploadFixture(t, "fresh bytes")})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.jsonPutCalls.Load() != 1 {
		t.Fatalf("JSON PUT calls = %d, want 1", srv.jsonPutCalls.Load())
	}
	got, _ := srv.putJSON["custom_html"].(string)
	if n := utf8.RuneCountInString(got); n != 400_000 {
		t.Fatalf("custom_html has %d characters, want 400000", n)
	}
}

func TestUpdate_CustomHTMLWithFileStillEnforcesTheCharacterCap(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "landing.html")
	if err := os.WriteFile(htmlPath, []byte(strings.Repeat("a", pageutil.MaxCustomHTMLLength+1)), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var calls atomic.Int32
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("an oversize page should be rejected before the request, not sent")
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1", "--custom-html", htmlPath, "--file", writeProductUploadFixture(t, "fresh bytes")})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want a local error for a page over the limit")
	}
	if calls.Load() != 0 {
		t.Fatalf("API was called %d times", calls.Load())
	}
	if !strings.Contains(err.Error(), "500000") {
		t.Fatalf("error should name the limit, got %q", err)
	}
}
