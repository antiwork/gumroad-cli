package pages

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func writePageHTML(t *testing.T, html string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(html), 0o600); err != nil {
		t.Fatalf("write html: %v", err)
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

func pageJSON(slug, title string, customHTML any) map[string]any {
	return map[string]any{
		"slug":        slug,
		"title":       title,
		"content":     nil,
		"custom_html": customHTML,
		"url":         "https://jane.gumroad.com/" + slug,
	}
}

// --- List ---

func TestList_RendersTable(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"pages": []map[string]any{
				pageJSON("about", "About", nil),
				pageJSON("faq", "FAQ", "<h1>FAQ</h1>"),
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/pages" {
		t.Errorf("got %s %s, want GET /pages", gotMethod, gotPath)
	}
	if !strings.Contains(out, "about") || !strings.Contains(out, "rich text") {
		t.Errorf("output missing rich text page: %q", out)
	}
	if !strings.Contains(out, "faq") || !strings.Contains(out, "custom HTML") {
		t.Errorf("output missing custom HTML page: %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "pages": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No pages yet") {
		t.Errorf("output missing empty message: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"pages":   []map[string]any{pageJSON("about", "About", nil)},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "about\tAbout\trich text\thttps://jane.gumroad.com/about") {
		t.Errorf("plain output wrong: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"pages":   []map[string]any{pageJSON("about", "About", nil)},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output not JSON: %v: %q", err, out)
	}
}

// --- Create ---

func TestCreate_TitleRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--title") {
		t.Fatalf("expected --title required, got: %v", err)
	}
}

func TestCreate_PostsTitleAndSlug(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "page": pageJSON("about", "About", nil)})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"--title", "About", "--slug", "about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/pages" {
		t.Errorf("got %s %s, want POST /pages", gotMethod, gotPath)
	}
	if gotForm.Get("title") != "About" || gotForm.Get("slug") != "about" {
		t.Errorf("got form %v", gotForm)
	}
	if gotForm.Has("custom_html") {
		t.Errorf("custom_html should be absent without a path, got form %v", gotForm)
	}
	if !strings.Contains(out, "Created page About") || !strings.Contains(out, "Live at https://jane.gumroad.com/about") {
		t.Errorf("output missing create summary: %q", out)
	}
}

func TestCreate_WithHTMLFile(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>About</h1>")

	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "page": pageJSON("about", "About", "<h1>About</h1>")})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"--title", "About", htmlPath})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotForm.Get("custom_html") != "<h1>About</h1>" {
		t.Errorf("got custom_html=%q", gotForm.Get("custom_html"))
	}
}

func TestCreate_MissingFileIsUsageError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"--title", "About", "/nonexistent/about.html"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("expected cannot-read usage error, got: %v", err)
	}
}

func TestCreate_TooManyArgs(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--title", "About", "a.html", "b.html"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("expected unexpected-argument error, got: %v", err)
	}
}

// --- Push ---

func TestPush_PutsHTMLToSluggedPage(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>About</h1>")

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "page": pageJSON("about", "About", "<h1>About</h1>")})
	})

	cmd := testutil.Command(newPushCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != "/pages/about" {
		t.Errorf("got %s %s, want PUT /pages/about", gotMethod, gotPath)
	}
	if gotForm.Get("custom_html") != "<h1>About</h1>" {
		t.Errorf("got custom_html=%q", gotForm.Get("custom_html"))
	}
	if !strings.Contains(out, "Published page About") {
		t.Errorf("output missing publish summary: %q", out)
	}
}

func TestPush_ProfileGoesThroughProfileEndpoint(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Home</h1>")

	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success":              true,
			"custom_html":          "<h1>Home</h1>",
			"previous_custom_html": nil,
			"profile_url":          "https://jane.gumroad.com",
			"sanitization_report":  emptyReport(),
		})
	})

	cmd := testutil.Command(newPushCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != "/user/custom_html" {
		t.Errorf("got %s %s, want PUT /user/custom_html", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Published page") || !strings.Contains(out, "Live at https://jane.gumroad.com") {
		t.Errorf("output missing publish summary: %q", out)
	}
}

func TestPush_ReadsStdin(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "page": pageJSON("about", "About", "<h1>From stdin</h1>")})
	})

	cmd := testutil.Command(newPushCmd(),
		testutil.Quiet(false), testutil.NoColor(true),
		testutil.Stdin(strings.NewReader("<h1>From stdin</h1>")))
	cmd.SetArgs([]string{"about", "-"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotForm.Get("custom_html") != "<h1>From stdin</h1>" {
		t.Errorf("got custom_html=%q from stdin", gotForm.Get("custom_html"))
	}
}

func TestPush_MissingSlug(t *testing.T) {
	cmd := newPushCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing page slug") {
		t.Fatalf("expected missing-slug error, got: %v", err)
	}
}

func TestPush_MissingPathIsUsageError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newPushCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing HTML path") {
		t.Fatalf("expected missing-path error, got: %v", err)
	}
}

func TestPush_MissingFileIsUsageError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newPushCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "/nonexistent/about.html"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("expected cannot-read usage error, got: %v", err)
	}
}

// --- Preview ---

func TestPreview_PostsToDryRunEndpoint(t *testing.T) {
	htmlPath := writePageHTML(t, "<h1>Draft</h1><script>alert(1)</script>")

	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"success":     true,
			"custom_html": "<h1>Draft</h1>",
			"sanitization_report": map[string]any{
				"removed_tags":       []map[string]any{{"tag": "script", "attrs": map[string]any{}, "reason": "disallowed tag"}},
				"removed_attributes": []any{},
				"total_removed":      1,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPreviewCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/user/preview_custom_html" {
		t.Errorf("got %s %s, want POST /user/preview_custom_html", gotMethod, gotPath)
	}
	if !strings.Contains(gotForm.Get("custom_html"), "<script>") {
		t.Errorf("original HTML should be sent unmodified, got %q", gotForm.Get("custom_html"))
	}
	if !strings.Contains(out, "Previewed page") || !strings.Contains(out, "script") {
		t.Errorf("output missing preview summary: %q", out)
	}
}

func TestPreview_MissingFileIsUsageError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newPreviewCmd(), testutil.NoColor(true))
	cmd.SetArgs([]string{"/nonexistent/draft.html"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("expected cannot-read usage error, got: %v", err)
	}
}
