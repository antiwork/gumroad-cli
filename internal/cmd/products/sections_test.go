package products

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestSectionsList_Table(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeSectionsProduct(t, w, 1)
	})

	cmd := testutil.Command(newSectionsListCmd())
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotPath != "/products/prod_123" {
		t.Fatalf("got path %q, want /products/prod_123", gotPath)
	}
	assertInOrder(t, out, "sec_intro", "sec_products", "sec_featured")
	for _, want := range []string{"#", "ID", "TYPE", "HEADER", "DETAILS", "1 *", "products=3 sort=newest", "featured=prod_z"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q: %q", want, out)
		}
	}
}

func TestSectionsList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"product": map[string]any{
				"id":                 "prod_123",
				"name":               "Art Pack",
				"sections":           sectionsFixture(),
				"main_section_index": 1,
			},
		})
	})

	cmd := testutil.Command(newSectionsListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if _, ok := resp["product"]; ok {
		t.Fatalf("JSON output should project sections, not the whole product: %q", out)
	}
	if _, ok := resp["name"]; ok {
		t.Fatalf("JSON output leaked product fields: %q", out)
	}
	if success, ok := resp["success"].(bool); !ok || !success {
		t.Fatalf("JSON output should include success=true, got: %q", out)
	}
	sections, ok := resp["sections"].([]any)
	if !ok || len(sections) != 3 {
		t.Fatalf("got sections=%#v, want three sections", resp["sections"])
	}
	if got := int(resp["main_section_index"].(float64)); got != 1 {
		t.Fatalf("got main_section_index=%d, want 1", got)
	}
}

func TestSectionsList_JSONPreservesExplicitFalseAndEmptyFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"product": map[string]any{
				"id": "prod_123",
				"sections": []map[string]any{
					{
						"id":                   "sec_products",
						"type":                 "products",
						"header":               "Products",
						"hide_header":          false,
						"shown_products":       []string{},
						"default_product_sort": "highest_rated",
						"show_filters":         false,
						"add_new_products":     false,
					},
				},
				"main_section_index": 0,
			},
		})
	})

	cmd := testutil.Command(newSectionsListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	var resp struct {
		Sections []map[string]any `json:"sections"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(resp.Sections))
	}
	section := resp.Sections[0]
	if _, ok := section["shown_products"]; !ok {
		t.Fatalf("expected shown_products key to be preserved, got: %q", out)
	}
	if _, ok := section["show_filters"]; !ok {
		t.Fatalf("expected show_filters key to be preserved, got: %q", out)
	}
	if _, ok := section["add_new_products"]; !ok {
		t.Fatalf("expected add_new_products key to be preserved, got: %q", out)
	}
	if section["show_filters"] != false || section["add_new_products"] != false {
		t.Fatalf("expected false flags to be preserved, got: %q", out)
	}
}

func TestSectionsList_JQFiltersProjectedJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		writeSectionsProduct(t, w, 2)
	})

	cmd := testutil.Command(newSectionsListCmd(), testutil.JQ(".main_section_index"))
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if strings.TrimSpace(out) != "2" {
		t.Fatalf("got jq output %q, want 2", out)
	}
}

func TestSectionsList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		writeSectionsProduct(t, w, 1)
	})

	cmd := testutil.Command(newSectionsListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	assertInOrder(t, out, "0\tsec_intro\ttext\tIntro\t-\t", "1\tsec_products\tproducts\tProducts\tproducts=3 sort=newest\tmain", "2\tsec_featured\tfeatured\tSpotlight\tfeatured=prod_z\t")
}

func TestSectionsList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"product": map[string]any{
				"id":                 "prod_123",
				"sections":           []any{},
				"main_section_index": 0,
			},
		})
	})

	cmd := testutil.Command(newSectionsListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})
	if !strings.Contains(out, "No sections found.") {
		t.Fatalf("expected empty message, got: %q", out)
	}
}

func TestSectionsList_MissingSectionsContract(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"product": map[string]any{
				"id":   "prod_123",
				"name": "Art Pack",
			},
		})
	})

	cmd := testutil.Command(newSectionsListCmd())
	cmd.SetArgs([]string{"prod_123"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing sections contract error")
	}
	if !strings.Contains(err.Error(), "product sections are not available") {
		t.Fatalf("expected sections contract error, got: %v", err)
	}
}

func TestSectionsList_NullSectionsRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"product": map[string]any{
				"id":                 "prod_123",
				"sections":           nil,
				"main_section_index": 0,
			},
		})
	})

	cmd := testutil.Command(newSectionsListCmd())
	cmd.SetArgs([]string{"prod_123"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid sections contract error")
	}
	if !strings.Contains(err.Error(), "sections must be an array") {
		t.Fatalf("expected sections array error, got: %v", err)
	}
}

func TestSectionsList_MissingProductID(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach API")
	})

	cmd := testutil.Command(newSectionsListCmd())
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing product id error")
	}
}

func TestSectionsList_InvalidProductID(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"success":false,"message":"Product not found"}`, http.StatusNotFound)
	})

	cmd := testutil.Command(newSectionsListCmd())
	cmd.SetArgs([]string{"missing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected API error for invalid product id")
	}
	if !strings.Contains(err.Error(), "Product not found") {
		t.Fatalf("expected product error, got: %v", err)
	}
}

func writeSectionsProduct(t *testing.T, w http.ResponseWriter, mainSectionIndex int) {
	t.Helper()
	testutil.JSON(t, w, map[string]any{
		"success": true,
		"product": map[string]any{
			"id":                 "prod_123",
			"sections":           sectionsFixture(),
			"main_section_index": mainSectionIndex,
		},
	})
}

func sectionsFixture() []map[string]any {
	return []map[string]any{
		{
			"id":          "sec_intro",
			"type":        "text",
			"header":      "Intro",
			"hide_header": false,
		},
		{
			"id":                   "sec_products",
			"type":                 "products",
			"header":               "Products",
			"hide_header":          false,
			"shown_products":       []string{"prod_a", "prod_b", "prod_c"},
			"default_product_sort": "newest",
			"show_filters":         true,
			"add_new_products":     true,
		},
		{
			"id":               "sec_featured",
			"type":             "featured",
			"header":           "Spotlight",
			"hide_header":      false,
			"featured_product": "prod_z",
		},
	}
}

func assertInOrder(t *testing.T, value string, needles ...string) {
	t.Helper()

	last := -1
	for _, needle := range needles {
		idx := strings.Index(value, needle)
		if idx < 0 {
			t.Fatalf("missing %q in %q", needle, value)
		}
		if idx < last {
			t.Fatalf("expected %q after previous needle in %q", needle, value)
		}
		last = idx
	}
}
