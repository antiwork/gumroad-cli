package pages

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestScaffold_RichTextPage(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered rich text</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/pages/about" {
		t.Errorf("got %s %s, want GET /pages/about", gotMethod, gotPath)
	}
	data, err := os.ReadFile("about.html")
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}
	if string(data) != "<h1>Rendered rich text</h1>" {
		t.Errorf("scaffolded file wrong: %q", data)
	}
	if !strings.Contains(out, "Scaffolded about → about.html") {
		t.Errorf("output missing scaffold confirmation: %q", out)
	}
	if !strings.Contains(out, "static snapshot, not a faithful copy") || !strings.Contains(out, "converts about to custom HTML") {
		t.Errorf("output missing conversion warning: %q", out)
	}
}

func TestScaffold_ProfileDefaultRender(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"custom_html":      "",
			"rendered_html":    "<h1>Default store</h1>",
			"has_landing_page": false,
			"profile_url":      "https://jane.gumroad.com",
		})
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/user/custom_html" {
		t.Errorf("got %s, want GET /user/custom_html", gotPath)
	}
	data, err := os.ReadFile("profile.html")
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}
	if string(data) != "<h1>Default store</h1>" {
		t.Errorf("scaffolded profile wrong: %q", data)
	}
	if !strings.Contains(out, "static snapshot, not a faithful copy") {
		t.Errorf("output missing conversion warning: %q", out)
	}
}

func TestScaffold_CustomHTMLPageSuggestsPull(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, customPageJSON("about", "About", "<h1>Custom</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "pages pull about") {
		t.Fatalf("want pull hint, got %v", err)
	}
	if _, statErr := os.Stat("about.html"); !os.IsNotExist(statErr) {
		t.Error("failed scaffold must not leave a file behind")
	}
}

func TestScaffold_ProfileWithCustomHTMLSuggestsPull(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"custom_html":      "<h1>My landing</h1>",
			"rendered_html":    "<h1>My landing</h1>",
			"has_landing_page": true,
			"profile_url":      "https://jane.gumroad.com",
		})
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "pages pull profile") {
		t.Fatalf("want pull hint, got %v", err)
	}
	if _, statErr := os.Stat("profile.html"); !os.IsNotExist(statErr) {
		t.Error("failed scaffold must not leave a file behind")
	}
}

func TestScaffold_EmptyRender(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, ""))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "nothing to scaffold") {
		t.Fatalf("want empty-render error, got %v", err)
	}
	if _, statErr := os.Stat("about.html"); !os.IsNotExist(statErr) {
		t.Error("empty render must not leave a file behind")
	}
}

func TestScaffold_NotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success": false, "message": "The page was not found."}`)
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"missing"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "page not found: missing") {
		t.Fatalf("want not-found error, got %v", err)
	}
	if _, statErr := os.Stat("missing.html"); !os.IsNotExist(statErr) {
		t.Error("not-found must not leave a file behind")
	}
}

func TestScaffold_RefusesOverwrite(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists error, got %v", err)
	}
	if reached {
		t.Error("overwrite refusal must happen before the API request")
	}
}

func TestScaffold_Stdout(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "-o", "-"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "<h1>Rendered</h1>" {
		t.Errorf("stdout output wrong: %q", out)
	}
	if _, err := os.Stat("about.html"); !os.IsNotExist(err) {
		t.Errorf("stdout mode must not write a file: %v", err)
	}
}

func TestScaffold_Plain(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "about\tabout.html") {
		t.Errorf("plain output wrong: %q", out)
	}
}

func TestScaffold_Quiet(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>Rendered</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(true), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Errorf("quiet mode must print nothing: %q", out)
	}
	if _, err := os.Stat("about.html"); err != nil {
		t.Errorf("quiet mode must still write the file: %v", err)
	}
}

func TestScaffold_ForceOverwrites(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", nil, "<h1>New render</h1>"))
	})

	cmd := testutil.Command(newScaffoldCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--force"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	data, err := os.ReadFile("about.html")
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}
	if string(data) != "<h1>New render</h1>" {
		t.Errorf("forced scaffold did not overwrite: %q", data)
	}
}
