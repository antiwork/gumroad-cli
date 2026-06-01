package products

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func productCategoriesHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/categories" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"categories": []map[string]any{
				{"id": 10, "name": "design", "label": "Design", "path": "design", "parent_id": nil},
				{"id": 11, "name": "ui-and-web", "label": "UI & Web", "path": "design/ui-and-web", "parent_id": 10},
				{"id": 12, "name": "figma", "label": "Figma", "path": "design/ui-and-web/figma", "parent_id": 11},
				{"id": 20, "name": "business-and-money", "label": "Business & Money", "path": "business-and-money", "parent_id": nil},
			},
		})
	}
}

func TestCategories_Table(t *testing.T) {
	testutil.Setup(t, productCategoriesHandler(t))

	cmd := newCategoriesCmd()
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	for _, want := range []string{"LABEL", "PATH", "ID", "Figma", "design/ui-and-web/figma", "12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestCategories_JSON(t *testing.T) {
	testutil.Setup(t, productCategoriesHandler(t))

	cmd := testutil.Command(newCategoriesCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	var resp productCategoriesResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Categories) != 4 {
		t.Fatalf("got %d categories, want 4", len(resp.Categories))
	}
	if resp.Categories[2].Path != "design/ui-and-web/figma" {
		t.Fatalf("unexpected category order or path: %+v", resp.Categories)
	}
}

func TestCategories_SearchFiltersByLabelOrPath(t *testing.T) {
	testutil.Setup(t, productCategoriesHandler(t))

	cmd := testutil.Command(newCategoriesCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--search", "ui-and-web"})
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	var resp productCategoriesResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Categories) != 2 {
		t.Fatalf("got %d categories, want 2: %+v", len(resp.Categories), resp.Categories)
	}
	for _, category := range resp.Categories {
		if !strings.Contains(category.Path, "ui-and-web") {
			t.Fatalf("search returned non-matching category: %+v", category)
		}
	}

	cmd = newCategoriesCmd()
	cmd.SetArgs([]string{"--search", "business"})
	out = testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})
	if strings.Contains(out, "Figma") || !strings.Contains(out, "Business & Money") {
		t.Fatalf("label search did not filter table output: %q", out)
	}
}

func TestCategories_JQ(t *testing.T) {
	testutil.Setup(t, productCategoriesHandler(t))

	cmd := testutil.Command(newCategoriesCmd(), testutil.JQ(".categories[] | select(.label == \"Figma\") | .path"))
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	if strings.TrimSpace(out) != `"design/ui-and-web/figma"` {
		t.Fatalf("unexpected jq output: %q", out)
	}
}
