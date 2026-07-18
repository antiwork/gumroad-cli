package pages

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func pullJSON(slug, title string, customHTML any, renderedHTML string) map[string]any {
	return map[string]any{
		"success":       true,
		"page":          pageJSON(slug, title, customHTML),
		"rendered_html": renderedHTML,
	}
}

func customPageJSON(slug, title, customHTML string) map[string]any {
	return pullJSON(slug, title, customHTML, customHTML)
}

func TestPull_WritesDefaultFile(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/pages/about" {
		t.Errorf("got %s %s, want GET /pages/about", gotMethod, gotPath)
	}
	data, err := os.ReadFile("about.html")
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if string(data) != "<h1>About</h1>" {
		t.Errorf("pulled file wrong: %q", data)
	}
	if !strings.Contains(out, "Pulled about → about.html") {
		t.Errorf("output missing pull confirmation: %q", out)
	}
	if !strings.Contains(out, "gumroad pages preview about.html") || !strings.Contains(out, "gumroad pages push about about.html") {
		t.Errorf("output missing next-steps loop: %q", out)
	}
}

func TestPull_OutputFlag(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "custom.html")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "-o", dest})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if string(data) != "<h1>About</h1>" {
		t.Errorf("pulled file wrong: %q", data)
	}
}

func TestPull_QuotesPathsWithSpaces(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "my page.html")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "-o", dest})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	quoted := "'" + dest + "'"
	if !strings.Contains(out, "gumroad pages preview "+quoted) || !strings.Contains(out, "gumroad pages push about "+quoted) {
		t.Errorf("paths with spaces must be quoted in suggested commands: %q", out)
	}
}

func TestPull_Stdout(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "-o", "-"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "<h1>About</h1>" {
		t.Errorf("stdout output wrong: %q", out)
	}
	if _, err := os.Stat("about.html"); !os.IsNotExist(err) {
		t.Errorf("stdout mode must not write a file: %v", err)
	}
}

func TestPull_JSONStillWritesFile(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output not JSON: %v: %q", err, out)
	}
	if resp["rendered_html"] != "<h1>About</h1>" {
		t.Errorf("JSON output missing rendered_html: %q", out)
	}
	data, err := os.ReadFile("about.html")
	if err != nil {
		t.Fatalf("JSON mode must still write the file: %v", err)
	}
	if string(data) != "<h1>About</h1>" {
		t.Errorf("pulled file wrong in JSON mode: %q", data)
	}
}

func TestPull_JSONWithStdoutRejected(t *testing.T) {
	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"about", "-o", "-"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("want incompatible-flags error, got %v", err)
	}
	if reached {
		t.Error("flag validation must happen before the API request")
	}
}

func TestPull_RefusesOverwrite(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists error, got %v", err)
	}
	if reached {
		t.Error("overwrite refusal must happen before the API request")
	}
	data, _ := os.ReadFile("about.html")
	if string(data) != "existing" {
		t.Errorf("existing file must be untouched: %q", data)
	}
}

func TestPull_RefusesOverwriteInJSONMode(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists error, got %v", err)
	}
	data, _ := os.ReadFile("about.html")
	if string(data) != "existing" {
		t.Errorf("existing file must be untouched: %q", data)
	}
}

func TestPull_ForceOverwrites(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>New</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--force"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	data, err := os.ReadFile("about.html")
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if string(data) != "<h1>New</h1>" {
		t.Errorf("forced pull did not overwrite: %q", data)
	}
}

func TestPull_FailedPullLeavesExistingFileUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	// A rich-text page fails extraction AFTER the download; even with --force
	// the destination must survive a failed pull.
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--force"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want error for rich-text page, got nil")
	}
	data, _ := os.ReadFile("about.html")
	if string(data) != "existing" {
		t.Errorf("existing file must survive a failed pull: %q", data)
	}
	leftovers, _ := filepath.Glob(".gumroad-pull-*")
	if len(leftovers) != 0 {
		t.Errorf("temp files must be cleaned up: %v", leftovers)
	}
}

func TestPull_Profile(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"custom_html":      "<h1>My landing</h1>",
			"rendered_html":    "<h1>My landing</h1>",
			"has_landing_page": true,
			"profile_url":      "https://jane.gumroad.com",
		})
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/user/custom_html" {
		t.Errorf("got %s %s, want GET /user/custom_html", gotMethod, gotPath)
	}
	data, err := os.ReadFile("profile.html")
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if string(data) != "<h1>My landing</h1>" {
		t.Errorf("pulled profile wrong: %q", data)
	}
	if !strings.Contains(out, "Pulled profile → profile.html") {
		t.Errorf("output missing pull confirmation: %q", out)
	}
}

func TestPull_ProfileWithoutCustomHTMLSuggestsScaffold(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"custom_html":      "",
			"rendered_html":    "<h1>Default store</h1>",
			"has_landing_page": false,
			"profile_url":      "https://jane.gumroad.com",
		})
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "pages scaffold profile") {
		t.Fatalf("want scaffold hint, got %v", err)
	}
	if _, statErr := os.Stat("profile.html"); !os.IsNotExist(statErr) {
		t.Error("failed pull must not leave a file behind")
	}
}

func TestPull_RichTextPageSuggestsScaffold(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered rich text</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "pages scaffold about") {
		t.Fatalf("want scaffold hint, got %v", err)
	}
	if _, statErr := os.Stat("about.html"); !os.IsNotExist(statErr) {
		t.Error("failed pull must not leave a file behind")
	}
}

func TestPull_NotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	// The API reports a missing page as HTTP 200 with success: false
	// (see Api::V2::PagesController#set_page), not as a 404.
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success": false, "message": "The page was not found."}`)
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"missing"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "page not found: missing") {
		t.Fatalf("want not-found error, got %v", err)
	}
	if _, statErr := os.Stat("missing.html"); !os.IsNotExist(statErr) {
		t.Error("not-found must not leave a file behind")
	}
}

func TestPull_InvalidSlugs(t *testing.T) {
	cases := []string{"weird/slug", "../backup/about", "About", "a_b", ""}
	for _, slug := range cases {
		t.Run(slug, func(t *testing.T) {
			reached := false
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				reached = true
				testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
			})

			cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
			cmd.SetArgs([]string{slug, "-o", filepath.Join(t.TempDir(), "out.html")})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "invalid page slug") {
				t.Fatalf("want invalid-slug error for %q, got %v", slug, err)
			}
			if reached {
				t.Error("slug validation must happen before the API request")
			}
		})
	}
}

func TestPull_ArgErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing slug", []string{}, "missing page slug"},
		{"extra arg", []string{"about", "extra"}, "unexpected argument: extra"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := testutil.Command(newPullCmd())
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestPull_Plain(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "about\tabout.html") {
		t.Errorf("plain output wrong: %q", out)
	}
}

func TestPull_Quiet(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(true), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Errorf("quiet mode must print nothing: %q", out)
	}
	if _, err := os.Stat("about.html"); err != nil {
		t.Errorf("quiet mode must still write the file: %v", err)
	}
}

func TestPull_FileAppearingDuringDownloadRefused(t *testing.T) {
	t.Chdir(t.TempDir())

	// The destination does not exist at the early check but appears while the
	// download is in flight (simulated by the test server handler). Without
	// --force, the write must be refused at replace time.
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile("about.html", []byte("raced"), 0o600); err != nil {
			t.Errorf("seed racing file: %v", err)
		}
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists error at replace time, got %v", err)
	}
	data, _ := os.ReadFile("about.html")
	if string(data) != "raced" {
		t.Errorf("racing file must be untouched: %q", data)
	}
	leftovers, _ := filepath.Glob(".gumroad-pull-*")
	if len(leftovers) != 0 {
		t.Errorf("temp files must be cleaned up: %v", leftovers)
	}
}
