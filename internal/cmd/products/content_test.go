package products

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestContentGet_JSONProjectsRichContent(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Errorf("got method %s, want GET", r.Method)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"product": map[string]any{
				"id":                                     "prod_123",
				"name":                                   "Art Pack",
				"has_same_rich_content_for_all_variants": true,
				"rich_content": []map[string]any{
					contentPage("page_1", "Start", 0),
					contentPage("page_2", "Files", 1),
				},
			},
		})
	})

	cmd := testutil.Command(newContentGetCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotPath != "/products/prod_123" {
		t.Fatalf("got path %q, want /products/prod_123", gotPath)
	}
	var pages []map[string]any
	if err := json.Unmarshal([]byte(out), &pages); err != nil {
		t.Fatalf("output is not rich_content JSON: %v\n%s", err, out)
	}
	if len(pages) != 2 || pages[0]["id"] != "page_1" || pages[1]["id"] != "page_2" {
		t.Fatalf("unexpected rich_content projection: %#v", pages)
	}
	if strings.Contains(out, "Art Pack") || strings.Contains(out, "product") {
		t.Fatalf("content get should not dump the whole product response: %s", out)
	}
}

func TestContentGet_PerVariantProductErrors(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, perVariantContentProductResponse())
	})

	cmd := testutil.Command(newContentGetCmd())
	cmd.SetArgs([]string{"prod_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "per-variant rich content") {
		t.Fatalf("expected per-variant guard error, got %v", err)
	}
	var invalidInputErr *cmdutil.InvalidInputError
	if !errors.As(err, &invalidInputErr) {
		t.Fatalf("expected invalid input error, got %T", err)
	}
}

func TestContentGet_OmittedRichContentPrintsEmptyArray(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sharedContentProductResponseWithoutRichContent())
	})

	cmd := testutil.Command(newContentGetCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("got omitted rich_content output %q, want []", out)
	}
}

func TestContentGet_VariantProjectsVariantRichContent(t *testing.T) {
	var productGetCalls, variantGetCalls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/prod_123":
			productGetCalls++
			if r.Method != http.MethodGet {
				t.Errorf("product got method %s, want GET", r.Method)
			}
			testutil.JSON(t, w, perVariantContentProductResponse())
		case "/products/prod_123/variant_categories/cat_123/variants/var_123":
			variantGetCalls++
			if r.Method != http.MethodGet {
				t.Errorf("variant got method %s, want GET", r.Method)
			}
			testutil.JSON(t, w, map[string]any{
				"success": true,
				"variant": map[string]any{
					"id":   "var_123",
					"name": "Large",
					"rich_content": []map[string]any{
						contentPage("variant_page_1", "Variant files", 0),
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	})

	cmd := testutil.Command(newContentGetCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123", "--variant", "var_123", "--category", "cat_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if productGetCalls != 1 || variantGetCalls != 1 {
		t.Fatalf("GET calls: product=%d variant=%d, want 1 each", productGetCalls, variantGetCalls)
	}
	var pages []map[string]any
	if err := json.Unmarshal([]byte(out), &pages); err != nil {
		t.Fatalf("output is not rich_content JSON: %v\n%s", err, out)
	}
	if len(pages) != 1 || pages[0]["id"] != "variant_page_1" {
		t.Fatalf("unexpected variant rich_content projection: %#v", pages)
	}
}

func TestContentGet_VariantRequiresCategoryBeforeAPI(t *testing.T) {
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API without category", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentGetCmd())
	cmd.SetArgs([]string{"prod_123", "--variant", "var_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--category") {
		t.Fatalf("expected missing category error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("missing category made %d API calls", calls)
	}
}

func TestContentGet_CategoryRequiresVariantBeforeAPI(t *testing.T) {
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API without variant", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentGetCmd())
	cmd.SetArgs([]string{"prod_123", "--category", "cat_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--category can only be used with --variant") {
		t.Fatalf("expected category-without-variant usage error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("missing variant made %d API calls", calls)
	}
}

func TestContentGet_VariantOnSharedProductErrorsBeforeVariantGET(t *testing.T) {
	var variantGetCalls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/prod_123":
			testutil.JSON(t, w, sharedContentProductResponse([]map[string]any{
				contentPage("page_1", "Start", 0),
			}))
		case "/products/prod_123/variant_categories/cat_123/variants/var_123":
			variantGetCalls++
			http.Error(w, "should not fetch variant content for shared product", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	})

	cmd := testutil.Command(newContentGetCmd())
	cmd.SetArgs([]string{"prod_123", "--variant", "var_123", "--category", "cat_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "shared rich content") {
		t.Fatalf("expected shared-content guard error, got %v", err)
	}
	if variantGetCalls != 0 {
		t.Fatalf("variant GET calls = %d, want 0", variantGetCalls)
	}
}

func TestContentGet_RejectsPlainOutput(t *testing.T) {
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API when output mode is invalid", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentGetCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "omit --plain or use --jq") {
		t.Fatalf("expected --plain usage error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("invalid output mode made %d API calls", calls)
	}
}

func TestContentHelpDocumentsWholeDocumentDeletion(t *testing.T) {
	var out strings.Builder
	cmd := newContentSetCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	text := out.String()
	for _, want := range []string{"whole-document write", "omitted from the JSON are deleted", "--dry-run", "--yes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("content set help missing %q: %q", want, text)
		}
	}
}

func TestContentSet_FileSendsWholeDocumentJSON(t *testing.T) {
	path := writeContentFixture(t, `[
  {"id":"page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}
]`)
	var putBody map[string]any
	var putContentType string

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.JSON(t, w, sharedContentProductResponse([]map[string]any{
				contentPage("page_1", "Start", 0),
				contentPage("page_2", "Old files", 1),
			}))
		case http.MethodPut:
			putContentType = r.Header.Get("Content-Type")
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			testutil.JSON(t, w, map[string]any{"success": true, "product": map[string]any{"id": "prod_123"}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod_123", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.HasPrefix(putContentType, "application/json") {
		t.Fatalf("got Content-Type %q, want application/json", putContentType)
	}
	pages := richContentPagesFromBody(t, putBody)
	if len(pages) != 1 || pages[0]["id"] != "page_1" {
		t.Fatalf("set should send exactly the provided document, got %#v", pages)
	}
}

func TestContentSet_VariantSendsWholeDocumentJSON(t *testing.T) {
	path := writeContentFixture(t, `[
  {"id":"variant_page_1","title":"Variant start","position":0,"description":{"type":"doc","content":[]}}
]`)
	var productPutCalls int
	var variantPutBody map[string]any
	var variantPutContentType string

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/prod_123":
			switch r.Method {
			case http.MethodGet:
				testutil.JSON(t, w, perVariantContentProductResponse())
			case http.MethodPut:
				productPutCalls++
				http.Error(w, "should not PUT product rich_content for variant target", http.StatusInternalServerError)
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		case "/products/prod_123/variant_categories/cat_123/variants/var_123":
			switch r.Method {
			case http.MethodGet:
				testutil.JSON(t, w, map[string]any{
					"success": true,
					"variant": map[string]any{
						"id": "var_123",
						"rich_content": []map[string]any{
							contentPage("variant_page_1", "Variant start", 0),
						},
					},
				})
			case http.MethodPut:
				variantPutContentType = r.Header.Get("Content-Type")
				if err := json.NewDecoder(r.Body).Decode(&variantPutBody); err != nil {
					t.Fatalf("decode variant PUT body: %v", err)
				}
				testutil.JSON(t, w, map[string]any{"success": true, "variant": map[string]any{"id": "var_123"}})
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod_123", path, "--variant", "var_123", "--category", "cat_123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if productPutCalls != 0 {
		t.Fatalf("product PUT calls = %d, want 0", productPutCalls)
	}
	if !strings.HasPrefix(variantPutContentType, "application/json") {
		t.Fatalf("got Content-Type %q, want application/json", variantPutContentType)
	}
	pages := richContentPagesFromBody(t, variantPutBody)
	if len(pages) != 1 || pages[0]["id"] != "variant_page_1" {
		t.Fatalf("variant set should send exactly the provided document, got %#v", pages)
	}
}

func TestContentSet_DeletingPageRequiresConfirmation(t *testing.T) {
	path := writeContentFixture(t, `[{"id":"page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}]`)
	var putCalls int

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.JSON(t, w, sharedContentProductResponse([]map[string]any{
				contentPage("page_1", "Start", 0),
				contentPage("page_2", "Old files", 1),
			}))
		case http.MethodPut:
			putCalls++
			http.Error(w, "should not PUT without confirmation", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"prod_123", path})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "Use --yes to skip confirmation") {
		t.Fatalf("expected confirmation-required error, got %v", err)
	}
	if putCalls != 0 {
		t.Fatalf("content set PUT %d times without confirmation", putCalls)
	}
}

func TestContentSet_VariantDeletingPageRequiresConfirmation(t *testing.T) {
	path := writeContentFixture(t, `[{"id":"variant_page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}]`)
	var variantPutCalls int

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/prod_123":
			testutil.JSON(t, w, perVariantContentProductResponse())
		case "/products/prod_123/variant_categories/cat_123/variants/var_123":
			switch r.Method {
			case http.MethodGet:
				testutil.JSON(t, w, map[string]any{
					"success": true,
					"variant": map[string]any{
						"id": "var_123",
						"rich_content": []map[string]any{
							contentPage("variant_page_1", "Start", 0),
							contentPage("variant_page_2", "Old files", 1),
						},
					},
				})
			case http.MethodPut:
				variantPutCalls++
				http.Error(w, "should not PUT without confirmation", http.StatusInternalServerError)
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"prod_123", path, "--variant", "var_123", "--category", "cat_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "Use --yes to skip confirmation") {
		t.Fatalf("expected confirmation-required error, got %v", err)
	}
	if variantPutCalls != 0 {
		t.Fatalf("variant content set PUT %d times without confirmation", variantPutCalls)
	}
}

func TestContentSet_ReadsStdinDash(t *testing.T) {
	var putBody map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.JSON(t, w, sharedContentProductResponse([]map[string]any{
				contentPage("page_1", "Start", 0),
			}))
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			testutil.JSON(t, w, map[string]any{"success": true})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	})

	stdin := bytes.NewBufferString(`[{"id":"page_1","title":"From stdin","position":0,"description":{"type":"doc","content":[]}}]`)
	cmd := testutil.Command(newContentSetCmd(), testutil.Stdin(stdin))
	cmd.SetArgs([]string{"prod_123", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	pages := richContentPagesFromBody(t, putBody)
	if pages[0]["title"] != "From stdin" {
		t.Fatalf("content set did not read stdin document: %#v", pages)
	}
}

func TestContentSet_DryRunJSONPreviewsPUTBodyAndSkipsPUT(t *testing.T) {
	path := writeContentFixture(t, `[{"id":"page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}]`)
	var putCalls int

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.JSON(t, w, sharedContentProductResponse([]map[string]any{
				contentPage("page_1", "Start", 0),
				contentPage("page_2", "Old files", 1),
			}))
		case http.MethodPut:
			putCalls++
			http.Error(w, "dry-run must not PUT", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123", path})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if putCalls != 0 {
		t.Fatalf("dry-run PUT %d times", putCalls)
	}
	var payload productContentSetDryRun
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output is not valid JSON: %v\n%s", err, out)
	}
	if !payload.DryRun || payload.Request.Method != http.MethodPut || payload.Request.Path != "/products/prod_123" {
		t.Fatalf("unexpected dry-run request: %+v", payload)
	}
	if len(payload.DeletedPageIDs) != 1 || payload.DeletedPageIDs[0] != "page_2" {
		t.Fatalf("dry-run should report omitted existing page IDs, got %#v", payload.DeletedPageIDs)
	}
	pages := richContentPagesFromBody(t, payload.Request.Body)
	if len(pages) != 1 || pages[0]["id"] != "page_1" {
		t.Fatalf("dry-run body should preview rich_content PUT body, got %#v", pages)
	}
}

func TestContentSet_VariantDryRunJSONPreviewsVariantPUTBodyAndSkipsPUT(t *testing.T) {
	path := writeContentFixture(t, `[{"id":"variant_page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}]`)
	var variantPutCalls int

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/prod_123":
			testutil.JSON(t, w, perVariantContentProductResponse())
		case "/products/prod_123/variant_categories/cat_123/variants/var_123":
			switch r.Method {
			case http.MethodGet:
				testutil.JSON(t, w, map[string]any{
					"success": true,
					"variant": map[string]any{
						"id": "var_123",
						"rich_content": []map[string]any{
							contentPage("variant_page_1", "Start", 0),
							contentPage("variant_page_2", "Old files", 1),
						},
					},
				})
			case http.MethodPut:
				variantPutCalls++
				http.Error(w, "dry-run must not PUT", http.StatusInternalServerError)
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123", path, "--variant", "var_123", "--category", "cat_123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if variantPutCalls != 0 {
		t.Fatalf("dry-run variant PUT %d times", variantPutCalls)
	}
	var payload productContentSetDryRun
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output is not valid JSON: %v\n%s", err, out)
	}
	if payload.Request.Method != http.MethodPut || payload.Request.Path != "/products/prod_123/variant_categories/cat_123/variants/var_123" {
		t.Fatalf("unexpected dry-run request: %+v", payload.Request)
	}
	if len(payload.DeletedPageIDs) != 1 || payload.DeletedPageIDs[0] != "variant_page_2" {
		t.Fatalf("dry-run should report omitted variant page IDs, got %#v", payload.DeletedPageIDs)
	}
	pages := richContentPagesFromBody(t, payload.Request.Body)
	if len(pages) != 1 || pages[0]["id"] != "variant_page_1" {
		t.Fatalf("dry-run body should preview variant rich_content PUT body, got %#v", pages)
	}
}

func TestContentSet_CategoryRequiresVariantBeforeFileReadOrAPI(t *testing.T) {
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API without variant", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentSetCmd())
	cmd.SetArgs([]string{"prod_123", filepath.Join(t.TempDir(), "missing.json"), "--category", "cat_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--category can only be used with --variant") {
		t.Fatalf("expected category-without-variant usage error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("missing variant made %d API calls", calls)
	}
}

func TestContentSet_PerVariantProductErrorsBeforePUT(t *testing.T) {
	path := writeContentFixture(t, `[{"id":"page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}]`)
	var putCalls int

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.JSON(t, w, perVariantContentProductResponse())
		case http.MethodPut:
			putCalls++
			http.Error(w, "should not PUT per-variant product content", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod_123", path})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "per-variant rich content") {
		t.Fatalf("expected per-variant guard error, got %v", err)
	}
	var invalidInputErr *cmdutil.InvalidInputError
	if !errors.As(err, &invalidInputErr) {
		t.Fatalf("expected invalid input error, got %T", err)
	}
	if putCalls != 0 {
		t.Fatalf("content set PUT %d times for per-variant product", putCalls)
	}
}

func TestContentSet_OmittedExistingRichContentActsEmpty(t *testing.T) {
	path := writeContentFixture(t, `[{"id":"page_1","title":"Start","position":0,"description":{"type":"doc","content":[]}}]`)
	var putCalls int

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.JSON(t, w, sharedContentProductResponseWithoutRichContent())
		case http.MethodPut:
			putCalls++
			http.Error(w, "dry-run must not PUT", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	})

	cmd := testutil.Command(newContentSetCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod_123", path})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if putCalls != 0 {
		t.Fatalf("dry-run PUT %d times", putCalls)
	}
	var payload productContentSetDryRun
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output is not valid JSON: %v\n%s", err, out)
	}
	if len(payload.DeletedPageIDs) != 0 {
		t.Fatalf("omitted existing rich_content should be empty, got deleted IDs %#v", payload.DeletedPageIDs)
	}
	pages := richContentPagesFromBody(t, payload.Request.Body)
	if len(pages) != 1 || pages[0]["id"] != "page_1" {
		t.Fatalf("dry-run body should keep provided content, got %#v", pages)
	}
}

func TestContentSet_InvalidDocumentDoesNotCallAPI(t *testing.T) {
	path := writeContentFixture(t, `{"id":"page_1"}`)
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API for invalid local JSON", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentSetCmd())
	cmd.SetArgs([]string{"prod_123", path})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "rich content JSON must be an array") {
		t.Fatalf("expected array validation error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("invalid document made %d API calls", calls)
	}
}

func TestContentSet_InvalidPageShapeDoesNotCallAPI(t *testing.T) {
	path := writeContentFixture(t, `[1, "a", true]`)
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API for invalid local page JSON", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentSetCmd())
	cmd.SetArgs([]string{"prod_123", path})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "rich content JSON page 0 must be an object") {
		t.Fatalf("expected page-shape validation error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("invalid page shape made %d API calls", calls)
	}
}

func TestContentSet_FileReadErrorDoesNotPrintUsage(t *testing.T) {
	var calls int
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not call API when local file cannot be read", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newContentSetCmd())
	cmd.SetArgs([]string{"prod_123", filepath.Join(t.TempDir(), "missing.json")})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("expected read error, got %v", err)
	}
	if strings.Contains(err.Error(), "Usage:") || strings.Contains(err.Error(), "--help") {
		t.Fatalf("read error should not include usage help, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("file read error made %d API calls", calls)
	}
}

func TestContentSet_DryRunPlainPreviewsPUTBody(t *testing.T) {
	var out bytes.Buffer
	opts := testutil.TestOptions(testutil.PlainOutput(), testutil.Stdout(&out))
	body := map[string]any{"rich_content": json.RawMessage(`[{"id":"page_1"}]`)}

	if err := renderProductContentSetDryRun(opts, "/products/prod_123", "content.json", nil, body); err != nil {
		t.Fatalf("renderProductContentSetDryRun failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "PUT\t/products/prod_123\t") || !strings.Contains(got, `"rich_content":[{"id":"page_1"}]`) {
		t.Fatalf("plain dry-run output should include PUT body, got %q", got)
	}
}

func TestContentSet_DryRunHumanReportsDeletedPages(t *testing.T) {
	var out bytes.Buffer
	opts := testutil.TestOptions(testutil.Stdout(&out), testutil.NoColor(true))
	body := map[string]any{"rich_content": json.RawMessage(`[{"id":"page_1"}]`)}
	deletedIDs := []string{"page_2", "page_3", "page_4", "page_5", "page_6", "page_7"}

	if err := renderProductContentSetDryRun(opts, "/products/prod_123", "content.json", deletedIDs, body); err != nil {
		t.Fatalf("renderProductContentSetDryRun failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Dry run: PUT /products/prod_123",
		"Deletes rich content pages: page_2, page_3, page_4, page_5, page_6, and 1 more",
		`"rich_content"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("human dry-run output missing %q: %q", want, got)
		}
	}
}

func TestContentSet_CancelledVariantJSONUsesVariantID(t *testing.T) {
	var out bytes.Buffer
	opts := testutil.TestOptions(testutil.JSONOutput(), testutil.Stdout(&out))
	target := productContentTarget{
		ProductID: "prod_123",
		VariantID: "var_123",
	}

	if err := printProductContentSetCancelled(opts, target); err != nil {
		t.Fatalf("printProductContentSetCancelled failed: %v", err)
	}

	var resp struct {
		Success   bool   `json:"success"`
		Cancelled bool   `json:"cancelled"`
		ID        string `json:"id"`
		Action    string `json:"action"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("cancelled variant content output is invalid JSON: %v\n%s", err, out.String())
	}
	if resp.Success {
		t.Fatal("cancelled variant content should report success=false")
	}
	if !resp.Cancelled {
		t.Fatal("cancelled variant content should report cancelled=true")
	}
	if resp.ID != "var_123" {
		t.Fatalf("cancelled variant content id = %q, want var_123", resp.ID)
	}
	if resp.Action != "set content for variant var_123 for product prod_123" {
		t.Fatalf("cancelled variant content action = %q", resp.Action)
	}
}

func TestProductContentHelpers(t *testing.T) {
	if got := productContentPath([]string{"prod_123"}); got != defaultProductContentPath {
		t.Fatalf("got default content path %q, want %q", got, defaultProductContentPath)
	}
	if got := productContentPath([]string{"prod_123", "custom.json"}); got != "custom.json" {
		t.Fatalf("got explicit content path %q, want custom.json", got)
	}

	productTarget := productContentTarget{ProductID: "prod_123"}
	if got := productTarget.mutationID(); got != "prod_123" {
		t.Fatalf("product mutation ID = %q, want prod_123", got)
	}
	variantTarget := productContentTarget{ProductID: "prod_123", VariantID: "var_123"}
	if got := variantTarget.mutationID(); got != "var_123" {
		t.Fatalf("variant mutation ID = %q, want var_123", got)
	}

	resp := productContentResponse{
		RichContent:                      json.RawMessage(`[{"id":"page_1"}]`),
		HasSameRichContentForAllVariants: true,
	}
	state := resp.state()
	if string(state.RichContent) != `[{"id":"page_1"}]` || !state.HasSameRichContentForAllVariants {
		t.Fatalf("top-level content response state mismatch: %+v", state)
	}

	normalized, err := normalizeProductRichContent(json.RawMessage("null"))
	if err != nil {
		t.Fatalf("normalizeProductRichContent(null) failed: %v", err)
	}
	if string(normalized) != "[]" {
		t.Fatalf("normalizeProductRichContent(null) = %s, want []", normalized)
	}
	normalized, err = normalizeProductRichContent(nil)
	if err != nil {
		t.Fatalf("normalizeProductRichContent(nil) failed: %v", err)
	}
	if string(normalized) != "[]" {
		t.Fatalf("normalizeProductRichContent(nil) = %s, want []", normalized)
	}
	if _, err := parseProductContentDocument([]byte("")); err == nil {
		t.Fatal("expected empty rich content input to error")
	}
}

func TestDeletedRichContentPageIDs_InvalidPageShape(t *testing.T) {
	if _, err := deletedRichContentPageIDs(json.RawMessage(`[1]`), json.RawMessage(`[]`)); err == nil || !strings.Contains(err.Error(), "existing rich_content is invalid") {
		t.Fatalf("expected existing page-shape error, got %v", err)
	}
	if _, err := deletedRichContentPageIDs(json.RawMessage(`[]`), json.RawMessage(`[1]`)); err == nil || !strings.Contains(err.Error(), "new rich_content is invalid") {
		t.Fatalf("expected new page-shape error, got %v", err)
	}
}

func writeContentFixture(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "content.json")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write content fixture: %v", err)
	}
	return path
}

func sharedContentProductResponse(richContent []map[string]any) map[string]any {
	return map[string]any{
		"success": true,
		"product": map[string]any{
			"id":                                     "prod_123",
			"has_same_rich_content_for_all_variants": true,
			"rich_content":                           richContent,
		},
	}
}

func sharedContentProductResponseWithoutRichContent() map[string]any {
	return map[string]any{
		"success": true,
		"product": map[string]any{
			"id":                                     "prod_123",
			"has_same_rich_content_for_all_variants": true,
		},
	}
}

func perVariantContentProductResponse() map[string]any {
	return map[string]any{
		"success": true,
		"product": map[string]any{
			"id":                                     "prod_123",
			"has_same_rich_content_for_all_variants": false,
			"rich_content":                           []map[string]any{},
			"variants": []map[string]any{
				{
					"options": []map[string]any{
						{"name": "Large"},
					},
				},
			},
		},
	}
}

func contentPage(id, title string, position int) map[string]any {
	return map[string]any{
		"id":       id,
		"title":    title,
		"position": position,
		"description": map[string]any{
			"type":    "doc",
			"content": []map[string]any{},
		},
	}
}

func richContentPagesFromBody(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	raw, ok := body["rich_content"].([]any)
	if !ok {
		t.Fatalf("rich_content body has wrong type: %T", body["rich_content"])
	}
	pages := make([]map[string]any, len(raw))
	for i, value := range raw {
		page, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("rich_content[%d] has wrong type: %T", i, value)
		}
		pages[i] = page
	}
	return pages
}
