package products

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageURLPrintsLandingURL(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":          "prod1",
				"landing_url": "https://seller.gumroad.com/l/prod1",
			},
		})
	})

	cmd := testutil.Command(newPageURLCmd())
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

func TestPageURLPlainPrintsLandingURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":          "prod1",
				"landing_url": "https://seller.gumroad.com/l/prod1",
			},
		})
	})

	cmd := testutil.Command(newPageURLCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "https://seller.gumroad.com/l/prod1" {
		t.Fatalf("got plain output %q", out)
	}
}

func TestProductsCommandIncludesPageNamespace(t *testing.T) {
	cmd := NewProductsCmd()
	found, _, err := cmd.Find([]string{"page", "preview"})
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if found == nil || found.Name() != "preview" {
		t.Fatalf("expected products page preview command, got %#v", found)
	}
}
