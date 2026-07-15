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

func pullJSON(slug, title, renderedHTML string) map[string]any {
	return map[string]any{
		"success":       true,
		"page":          pageJSON(slug, title, nil),
		"rendered_html": renderedHTML,
	}
}

func TestPull_WritesDefaultFile(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>About</h1>"))
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
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>About</h1>"))
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

func TestPull_Stdout(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>About</h1>"))
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

func TestPull_JSON(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>About</h1>"))
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
	if _, err := os.Stat("about.html"); !os.IsNotExist(err) {
		t.Errorf("JSON mode must not write a file: %v", err)
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
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>About</h1>"))
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

func TestPull_ForceOverwrites(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("about.html", []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>New</h1>"))
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

func TestPull_Profile(t *testing.T) {
	t.Chdir(t.TempDir())

	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"custom_html":      "",
			"rendered_html":    "<h1>Store</h1>",
			"has_landing_page": false,
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
	if string(data) != "<h1>Store</h1>" {
		t.Errorf("pulled profile wrong: %q", data)
	}
	if !strings.Contains(out, "Pulled profile → profile.html") {
		t.Errorf("output missing pull confirmation: %q", out)
	}
}

func TestPull_NotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		testutil.RawJSON(t, w, `{"success": false, "message": "Page not found"}`)
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

func TestPull_EmptyRenderedHTML(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", ""))
	})

	cmd := testutil.Command(newPullCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no rendered HTML") {
		t.Fatalf("want empty-render error, got %v", err)
	}
	if _, statErr := os.Stat("about.html"); !os.IsNotExist(statErr) {
		t.Error("empty render must not leave a file behind")
	}
}

func TestPull_Plain(t *testing.T) {
	t.Chdir(t.TempDir())

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, pullJSON("about", "About", "<h1>About</h1>"))
	})

	cmd := testutil.Command(newPullCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "about\tabout.html") {
		t.Errorf("plain output wrong: %q", out)
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
