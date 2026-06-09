package products

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestURL_PrintsShortURL(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":        "prod1",
				"short_url": "https://seller.gumroad.com/l/prod1",
			},
		})
	})

	cmd := testutil.Command(newURLCmd())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet {
		t.Errorf("got method %q, want GET", gotMethod)
	}
	if gotPath != "/products/prod1" {
		t.Errorf("got path %q, want /products/prod1", gotPath)
	}
	if strings.TrimSpace(out) != "https://seller.gumroad.com/l/prod1" {
		t.Fatalf("got output %q", out)
	}
}

func TestURL_PlainPrintsShortURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":        "prod1",
				"short_url": "https://seller.gumroad.com/l/prod1",
			},
		})
	})

	cmd := testutil.Command(newURLCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "https://seller.gumroad.com/l/prod1" {
		t.Fatalf("got plain output %q", out)
	}
}

func TestURL_FallsBackToLandingURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":          "prod1",
				"landing_url": "https://seller.gumroad.com/l/prod1",
			},
		})
	})

	cmd := testutil.Command(newURLCmd())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "https://seller.gumroad.com/l/prod1" {
		t.Fatalf("got output %q", out)
	}
}

func TestURL_MissingShareLinkErrors(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "prod1"},
		})
	})

	cmd := testutil.Command(newURLCmd())
	cmd.SetArgs([]string{"prod1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when product response lacks a share link")
	}
	if !strings.Contains(err.Error(), "share link") {
		t.Fatalf("got error %q, want it to mention share link", err.Error())
	}
}
