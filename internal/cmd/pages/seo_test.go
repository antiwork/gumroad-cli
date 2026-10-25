package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func seoPageJSON(seoTitle, seoDescription any) map[string]any {
	page := pageJSON("about", "About", nil)
	page["seo_title"] = seoTitle
	page["seo_description"] = seoDescription
	return page
}

func TestSeo_ShowsPageFieldsWithDefaults(t *testing.T) {
	var gotMethod, gotPath, gotSourceOnly string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotSourceOnly = r.URL.Query().Get("source_only")
		testutil.JSON(t, w, map[string]any{"success": true, "page": seoPageJSON("About Jane", nil)})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/pages/about" || gotSourceOnly != "true" {
		t.Errorf("got %s %s source_only=%q, want GET /pages/about source_only=true", gotMethod, gotPath, gotSourceOnly)
	}
	if !strings.Contains(out, "Search title: About Jane") || !strings.Contains(out, "Search description: (default)") {
		t.Errorf("output missing search fields: %q", out)
	}
}

func TestSeo_SendsOnlyTheFlagsGiven(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "page": seoPageJSON(nil, "Who I am.")})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--title", "", "--description", "Who I am."})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != "/pages/about" {
		t.Errorf("got %s %s, want PUT /pages/about", gotMethod, gotPath)
	}
	if title, ok := gotForm["seo_title"]; !ok || title[0] != "" {
		t.Errorf("want an empty seo_title to clear the field, got %v", gotForm)
	}
	if gotForm.Get("seo_description") != "Who I am." {
		t.Errorf("got seo_description=%q", gotForm.Get("seo_description"))
	}
	if !strings.Contains(out, "Search title: (default)") {
		t.Errorf("output missing cleared title: %q", out)
	}
}

func TestSeo_LeavesAnOmittedFlagOut(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "page": seoPageJSON("About Jane", nil)})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--title", "About Jane"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if _, ok := gotForm["seo_description"]; ok {
		t.Errorf("an omitted flag must not be sent, got %v", gotForm)
	}
}

func TestSeo_ProfileGoesThroughUserEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"success": true, "user": map[string]any{"name": "Jane", "seo_title": "Hand-lettered posters", "seo_description": nil}})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile", "--title", "Hand-lettered posters"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != "/user" {
		t.Errorf("got %s %s, want PUT /user", gotMethod, gotPath)
	}
	if gotForm.Get("seo_title") != "Hand-lettered posters" {
		t.Errorf("got seo_title=%q", gotForm.Get("seo_title"))
	}
	if !strings.Contains(out, "Search title: Hand-lettered posters") {
		t.Errorf("output missing search title: %q", out)
	}
}

func TestSeo_ShowsProfileFields(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{"success": true, "user": map[string]any{"seo_title": nil, "seo_description": "Posters and prints."}})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"profile"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/user" {
		t.Errorf("got %s %s, want GET /user", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Search description: Posters and prints.") {
		t.Errorf("output missing search description: %q", out)
	}
}

func TestSeo_QuietStillPrintsARead(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "page": seoPageJSON("About Jane", nil)})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(true), testutil.NoColor(true))
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Search title: About Jane") {
		t.Errorf("a quiet read must still print the fields, got %q", out)
	}
}

func TestSeo_QuietSilencesASave(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "page": seoPageJSON("About Jane", nil)})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(true), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--title", "About Jane"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Errorf("a quiet save must print nothing, got %q", out)
	}
}

func TestSeo_MissingScopeGetsReauthHint(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		testutil.RawJSON(t, w, `{"success": false, "message": "This endpoint requires the edit_profile scope."}`)
	})

	cmd := testutil.Command(newSeoCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"about", "--title", "About Jane"})
	err := cmd.Execute()
	hinted, ok := err.(interface{ GetHint() string })
	if !ok || !strings.Contains(hinted.GetHint(), "gumroad auth login") {
		t.Fatalf("expected re-auth hint, got: %v", err)
	}
}

func TestSeo_PlainOutputPrintsSlugTitleAndDescription(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "page": seoPageJSON("About Jane", nil)})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"about"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "about\tAbout Jane\t\n" {
		t.Errorf("got %q, want slug, title and an empty description column", out)
	}
}

func TestSeo_PlainOutputForProfilePrintsBothFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "user": map[string]any{"seo_title": "Hand-lettered posters", "seo_description": "Posters and prints."}})
	})

	cmd := testutil.Command(newSeoCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"profile"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "profile\tHand-lettered posters\tPosters and prints.\n" {
		t.Errorf("got %q, want slug, title and description columns in that order", out)
	}
}
