package variants

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/richcontent"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func variantsHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"variants": []map[string]any{
				{"id": "v1", "name": "Large", "price_difference_cents": 500, "max_purchase_count": 0},
				{"id": "v2", "name": "XL", "price_difference_cents": 1000, "max_purchase_count": 10},
			},
		})
	}
}

func variantHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"variant": map[string]any{
				"id": "v1", "name": "Large", "description": "The large size",
				"price_difference_cents": 500, "max_purchase_count": 5,
			},
		})
	}
}

type variantFileAttachServers struct {
	s3 *httptest.Server

	sharedContent      bool
	productJSON        map[string]any
	variantJSON        map[string]any
	variantRichContent []map[string]any

	productGetCalls atomic.Int32
	productPutCalls atomic.Int32
	variantGetCalls atomic.Int32
	variantPutCalls atomic.Int32
	s3Calls         atomic.Int32
	completeSeq     atomic.Int32
}

func newVariantFileAttachServers(t *testing.T) *variantFileAttachServers {
	t.Helper()

	s := &variantFileAttachServers{
		variantRichContent: []map[string]any{{
			"id":    "page_1",
			"title": "Existing page",
			"description": map[string]any{
				"type": "doc",
				"content": []any{
					map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_existing", "uid": "old-uid"}},
					map[string]any{"type": "paragraph"},
				},
			},
		}},
	}
	s.s3 = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.s3Calls.Add(1)
		if r.Method != http.MethodPut {
			t.Errorf("S3 got %s, want PUT", r.Method)
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.s3.Close)

	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = s.s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

	return s
}

func (s *variantFileAttachServers) dispatch(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/p1":
			switch r.Method {
			case http.MethodGet:
				s.productGetCalls.Add(1)
				testutil.JSON(t, w, map[string]any{
					"product": map[string]any{
						"id": "p1",
						"files": []map[string]any{
							{"id": "file_existing", "name": "Existing.pdf"},
						},
						"has_same_rich_content_for_all_variants": s.sharedContent,
					},
				})
			case http.MethodPut:
				s.productPutCalls.Add(1)
				if err := json.NewDecoder(r.Body).Decode(&s.productJSON); err != nil {
					t.Fatalf("decode product JSON body: %v", err)
				}
				testutil.JSON(t, w, map[string]any{"product": map[string]any{"id": "p1"}})
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		case "/products/p1/variant_categories/vc1/variants/v1":
			switch r.Method {
			case http.MethodGet:
				s.variantGetCalls.Add(1)
				testutil.JSON(t, w, map[string]any{
					"variant": map[string]any{
						"id":           "v1",
						"name":         "Large",
						"rich_content": s.variantRichContent,
					},
				})
			case http.MethodPut:
				s.variantPutCalls.Add(1)
				if err := json.NewDecoder(r.Body).Decode(&s.variantJSON); err != nil {
					t.Fatalf("decode variant JSON body: %v", err)
				}
				testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1"}})
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		case "/files/presign":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm failed: %v", err)
			}
			testutil.JSON(t, w, map[string]any{
				"upload_id": "up-1",
				"key":       "attachments/u/k/original/upload.bin",
				"file_url":  "https://example.com/attachments/u/k/original/upload.bin",
				"parts": []map[string]any{
					{"part_number": 1, "presigned_url": s.s3.URL + "/part/1"},
				},
			})
		case "/files/complete":
			seq := s.completeSeq.Add(1)
			testutil.JSON(t, w, map[string]any{
				"file_url": fmt.Sprintf("https://example.com/attachments/u/k/original/upload-%d.bin", seq),
			})
		case "/files/abort":
			testutil.JSON(t, w, map[string]any{})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

func writeVariantUploadFixture(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func variantUpdateJSONFiles(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	raw, ok := body["files"].([]any)
	if !ok {
		t.Fatalf("files payload has wrong type: %T", body["files"])
	}
	files := make([]map[string]any, len(raw))
	for i, current := range raw {
		file, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("files[%d] has wrong type: %T", i, current)
		}
		files[i] = file
	}
	return files
}

func variantUpdateJSONRichContent(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	rawRichContent, ok := body["rich_content"].([]any)
	if !ok {
		t.Fatalf("variant rich_content has wrong type: %T", body["rich_content"])
	}
	richContentPages := make([]map[string]any, len(rawRichContent))
	for i, raw := range rawRichContent {
		page, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("rich_content[%d] has wrong type: %T", i, raw)
		}
		richContentPages[i] = page
	}
	return richContentPages
}

func TestNewVariantsCmdIncludesSubcommands(t *testing.T) {
	cmd := NewVariantsCmd()
	if cmd.Use != "variants" {
		t.Fatalf("Use = %q, want variants", cmd.Use)
	}

	names := map[string]bool{}
	for _, subcmd := range cmd.Commands() {
		names[subcmd.Name()] = true
	}
	for _, name := range []string{"list", "view", "create", "update", "delete"} {
		if !names[name] {
			t.Fatalf("missing subcommand %q", name)
		}
	}
}

// --- List ---

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"variants": []map[string]any{
				{"id": "v1", "name": "Large", "price_difference_cents": 500, "max_purchase_count": 0},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/products/p1/variant_categories/vc1/variants" {
		t.Errorf("got path %q", gotPath)
	}
	if !strings.Contains(out, "Large") {
		t.Errorf("output missing variant name: %q", out)
	}
	if !strings.Contains(out, "unlimited") {
		t.Errorf("max_purchase_count=0 should show 'unlimited': %q", out)
	}
}

func TestList_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--category", "vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required error, got: %v", err)
	}
}

func TestList_CategoryRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--category") {
		t.Fatalf("expected --category required error, got: %v", err)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, variantsHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	variants := resp["variants"].([]any)
	if len(variants) != 2 {
		t.Errorf("got %d variants, want 2", len(variants))
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, variantsHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "v1") || !strings.Contains(out, "Large") {
		t.Errorf("plain output missing data: %q", out)
	}
	if !strings.Contains(out, "v1\tLarge\t500\tunlimited") {
		t.Errorf("plain output missing unlimited max purchases: %q", out)
	}
	if !strings.Contains(out, "v2\tXL\t1000\t10") {
		t.Errorf("plain output missing max purchases column: %q", out)
	}
}

func TestList_NoColorDisablesANSI(t *testing.T) {
	testutil.Setup(t, variantsHandler(t))
	testutil.SetStdoutIsTerminal(t, true)

	cmd := testutil.Command(newListCmd(), testutil.NoColor(true))
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	testutil.AssertNoANSI(t, out)
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variants": []map[string]any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No variants found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestList_MaxPurchaseCount(t *testing.T) {
	testutil.Setup(t, variantsHandler(t))

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	// v2 has max_purchase_count=10
	if !strings.Contains(out, "10") {
		t.Errorf("expected max purchase count 10: %q", out)
	}
}

// --- View ---

func TestView_Table(t *testing.T) {
	testutil.Setup(t, variantHandler(t))

	cmd := newViewCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Large") {
		t.Errorf("missing name: %q", out)
	}
	if !strings.Contains(out, "500") {
		t.Errorf("missing price diff: %q", out)
	}
	if !strings.Contains(out, "Max purchases: 5") {
		t.Errorf("missing max purchases: %q", out)
	}
	if !strings.Contains(out, "The large size") {
		t.Errorf("missing description: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, variantHandler(t))

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, variantHandler(t))

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "v1") || !strings.Contains(out, "Large") {
		t.Errorf("plain missing data: %q", out)
	}
}

func TestView_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"v1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required error, got: %v", err)
	}
}

func TestView_CategoryRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--category") {
		t.Fatalf("expected --category required error, got: %v", err)
	}
}

func TestView_NoDescription(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"variant": map[string]any{
				"id": "v1", "name": "Small", "description": "",
				"price_difference_cents": 0, "max_purchase_count": 0,
			},
		})
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Small") {
		t.Errorf("missing name: %q", out)
	}
	// Should NOT show max purchases or description
	if strings.Contains(out, "Max purchases") {
		t.Errorf("should not show max purchases when 0: %q", out)
	}
}

func TestView_IntegerLikeFloats(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"variant": {
				"id": "v1",
				"name": "Large",
				"description": "The large size",
				"price_difference_cents": 500.0,
				"max_purchase_count": 5.0
			}
		}`)
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Price difference: 500 cents") || !strings.Contains(out, "Max purchases: 5") {
		t.Fatalf("output missing float-backed fields: %q", out)
	}
}

// --- Create ---

func TestCreate_Flags(t *testing.T) {
	var gotName, gotDesc, gotPriceDiff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotName = r.PostForm.Get("name")
		gotDesc = r.PostForm.Get("description")
		gotPriceDiff = r.PostForm.Get("price_difference_cents")
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": r.PostForm.Get("name")}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL", "--description", "Extra large", "--price-difference", "3.00"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotName != "XL" {
		t.Errorf("got name=%q, want XL", gotName)
	}
	if gotDesc != "Extra large" {
		t.Errorf("got description=%q, want 'Extra large'", gotDesc)
	}
	if gotPriceDiff != "300" {
		t.Errorf("got price_difference_cents=%q, want 300", gotPriceDiff)
	}
}

func TestCreate_PriceDifference(t *testing.T) {
	var gotPriceDiff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPriceDiff = r.PostForm.Get("price_difference_cents")
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": "XL"}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL", "--price-difference", "5.00"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPriceDiff != "500" {
		t.Errorf("got price_difference_cents=%q, want 500", gotPriceDiff)
	}
}

func TestCreate_PriceDifferenceNegative(t *testing.T) {
	var gotPriceDiff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPriceDiff = r.PostForm.Get("price_difference_cents")
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": "SM"}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "SM", "--price-difference", "-1.50"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPriceDiff != "-150" {
		t.Errorf("got price_difference_cents=%q, want -150", gotPriceDiff)
	}
}

func TestCreate_PriceDifferenceInvalidInput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL", "--price-difference", "abc"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a valid price") {
		t.Fatalf("expected validation error, got: %v", err)
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *cmdutil.UsageError, got %T", err)
	}
}

func TestCreate_NameRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected --name required error, got: %v", err)
	}
}

func TestCreate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--category", "vc1", "--name", "XL"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required error, got: %v", err)
	}
}

func TestCreate_CategoryRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "XL"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--category") {
		t.Fatalf("expected --category required error, got: %v", err)
	}
}

func TestCreate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestCreate_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": "XL"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "Created variant:") {
		t.Errorf("expected created message, got: %q", out)
	}
	if !strings.Contains(out, "v1") {
		t.Errorf("expected variant ID in output, got: %q", out)
	}
	if !strings.Contains(out, "XL") {
		t.Errorf("expected variant name in output, got: %q", out)
	}
}

func TestCreate_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": "XL"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "v1\tXL") {
		t.Errorf("expected plain tab-separated output, got: %q", out)
	}
}

func TestCreate_RejectsNegativeMaxPurchaseCount(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL", "--max-purchase-count", "-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-purchase-count cannot be negative") {
		t.Fatalf("expected max purchase validation error, got: %v", err)
	}
}

func TestCreate_MaxPurchaseCount(t *testing.T) {
	var gotParam string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotParam = r.PostForm.Get("max_purchase_count")
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": "XL"}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL", "--max-purchase-count", "50"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotParam != "50" {
		t.Errorf("got max_purchase_count=%q, want 50", gotParam)
	}
}

func TestCreate_MaxPurchaseCountZero(t *testing.T) {
	var gotParam string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotParam = r.PostForm.Get("max_purchase_count")
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1", "name": "XL"}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL", "--max-purchase-count", "0"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotParam != "0" {
		t.Errorf("got max_purchase_count=%q, want 0", gotParam)
	}
}

func TestCreate_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		testutil.JSON(t, w, map[string]any{"message": "Invalid"})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--category", "vc1", "--name", "XL"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Update ---

func TestUpdate_Flags(t *testing.T) {
	var gotName, gotPriceDiff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotName = r.PostForm.Get("name")
		gotPriceDiff = r.PostForm.Get("price_difference_cents")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--name", "XXL", "--price-difference", "7.00"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotName != "XXL" {
		t.Errorf("got name=%q, want XXL", gotName)
	}
	if gotPriceDiff != "700" {
		t.Errorf("got price_difference_cents=%q, want 700", gotPriceDiff)
	}
}

func TestUpdate_PriceDifference(t *testing.T) {
	var gotPriceDiff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPriceDiff = r.PostForm.Get("price_difference_cents")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--price-difference", "7.00"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPriceDiff != "700" {
		t.Errorf("got price_difference_cents=%q, want 700", gotPriceDiff)
	}
}

func TestUpdate_PriceDifferenceNegative(t *testing.T) {
	var gotPriceDiff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPriceDiff = r.PostForm.Get("price_difference_cents")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--price-difference", "-2.50"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPriceDiff != "-250" {
		t.Errorf("got price_difference_cents=%q, want -250", gotPriceDiff)
	}
}

func TestUpdate_PriceDifferenceInvalidInput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--price-difference", "$5"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a valid price") {
		t.Fatalf("expected validation error, got: %v", err)
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *cmdutil.UsageError, got %T", err)
	}
}

func TestUpdate_PriceDifferenceSatisfiesRequireAnyFlag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--price-difference", "3.00"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
}

func TestUpdate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required error, got: %v", err)
	}
}

func TestUpdate_CategoryRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--category") {
		t.Fatalf("expected --category required error, got: %v", err)
	}
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one field to update must be provided") {
		t.Fatalf("expected no-op update error, got: %v", err)
	}
}

func TestUpdate_RejectsNegativeMaxPurchaseCount(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--max-purchase-count", "-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-purchase-count cannot be negative") {
		t.Fatalf("expected max purchase validation error, got: %v", err)
	}
}

func TestUpdate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant": map[string]any{"id": "v1"}})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--name", "XXL"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestUpdate_Description(t *testing.T) {
	var gotDesc string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotDesc = r.PostForm.Get("description")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--description", "New desc"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotDesc != "New desc" {
		t.Errorf("got description=%q, want 'New desc'", gotDesc)
	}
}

func TestUpdate_ClearsDescription(t *testing.T) {
	var gotDesc string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotDesc = r.PostForm.Get("description")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--description", ""})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotDesc != "" {
		t.Errorf("got description=%q, want empty string", gotDesc)
	}
}

func TestUpdate_MaxPurchaseCount(t *testing.T) {
	var gotParam string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotParam = r.PostForm.Get("max_purchase_count")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--max-purchase-count", "25"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotParam != "25" {
		t.Errorf("got max_purchase_count=%q, want 25", gotParam)
	}
}

func TestUpdate_MaxPurchaseCountZero(t *testing.T) {
	var gotParam string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotParam = r.PostForm.Get("max_purchase_count")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1", "--max-purchase-count", "0"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotParam != "0" {
		t.Errorf("got max_purchase_count=%q, want 0", gotParam)
	}
}

func TestUpdate_FileAttachesToVariantRichContent(t *testing.T) {
	srv := newVariantFileAttachServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeVariantUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"v1",
		"--product", "p1",
		"--category", "vc1",
		"--file", path,
		"--file-name", "License.pdf",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.productPutCalls.Load() != 1 {
		t.Fatalf("product PUT calls = %d, want 1", srv.productPutCalls.Load())
	}
	if srv.variantPutCalls.Load() != 1 {
		t.Fatalf("variant PUT calls = %d, want 1", srv.variantPutCalls.Load())
	}
	if srv.s3Calls.Load() != 1 {
		t.Fatalf("S3 calls = %d, want 1", srv.s3Calls.Load())
	}
	if _, ok := srv.productJSON["rich_content"]; ok {
		t.Fatalf("product update unexpectedly sent rich_content: %#v", srv.productJSON["rich_content"])
	}

	files := variantUpdateJSONFiles(t, srv.productJSON)
	if len(files) != 2 {
		t.Fatalf("files payload len = %d, want 2", len(files))
	}
	if files[0]["id"] != "file_existing" {
		t.Fatalf("files[0].id = %#v, want file_existing", files[0]["id"])
	}
	newFileID, ok := files[1]["external_id"].(string)
	if !ok || !strings.HasPrefix(newFileID, "cli-upload-") {
		t.Fatalf("files[1].external_id = %#v, want generated id", files[1]["external_id"])
	}
	if files[1]["url"] != "https://example.com/attachments/u/k/original/upload-1.bin" {
		t.Fatalf("files[1].url = %#v", files[1]["url"])
	}

	richContentPages := variantUpdateJSONRichContent(t, srv.variantJSON)
	if ids := richcontent.FileEmbedIDs(richContentPages); !reflect.DeepEqual(ids, []string{"file_existing", newFileID}) {
		t.Fatalf("variant rich_content fileEmbed ids = %#v, want existing file then new upload", ids)
	}
}

func TestUpdate_FileCreatesVariantRichContentWhenEmpty(t *testing.T) {
	srv := newVariantFileAttachServers(t)
	srv.variantRichContent = []map[string]any{}
	testutil.Setup(t, srv.dispatch(t))

	path := writeVariantUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"v1",
		"--product", "p1",
		"--category", "vc1",
		"--file", path,
		"--file-name", "License.pdf",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := variantUpdateJSONFiles(t, srv.productJSON)
	if len(files) != 2 {
		t.Fatalf("files payload len = %d, want 2", len(files))
	}
	if files[0]["id"] != "file_existing" {
		t.Fatalf("files[0].id = %#v, want file_existing", files[0]["id"])
	}
	newFileID, ok := files[1]["external_id"].(string)
	if !ok || !strings.HasPrefix(newFileID, "cli-upload-") {
		t.Fatalf("files[1].external_id = %#v, want generated id", files[1]["external_id"])
	}

	richContentPages := variantUpdateJSONRichContent(t, srv.variantJSON)
	if ids := richcontent.FileEmbedIDs(richContentPages); !reflect.DeepEqual(ids, []string{newFileID}) {
		t.Fatalf("variant rich_content fileEmbed ids = %#v, want new upload only", ids)
	}
}

func TestUpdate_FileRejectsSharedContentBeforeUpload(t *testing.T) {
	srv := newVariantFileAttachServers(t)
	srv.sharedContent = true
	testutil.Setup(t, srv.dispatch(t))

	path := writeVariantUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"v1",
		"--product", "p1",
		"--category", "vc1",
		"--file", path,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected shared content error")
	}
	if !strings.Contains(err.Error(), "shared content") || !strings.Contains(err.Error(), "products update") {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.variantGetCalls.Load() != 0 {
		t.Fatalf("variant GET calls = %d, want 0", srv.variantGetCalls.Load())
	}
	if srv.s3Calls.Load() != 0 {
		t.Fatalf("S3 calls = %d, want 0", srv.s3Calls.Load())
	}
	if srv.productPutCalls.Load() != 0 || srv.variantPutCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: product=%d variant=%d", srv.productPutCalls.Load(), srv.variantPutCalls.Load())
	}
}

func TestUpdate_FileDryRunJSONShowsProductAndVariantRequests(t *testing.T) {
	srv := newVariantFileAttachServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeVariantUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{
		"v1",
		"--product", "p1",
		"--category", "vc1",
		"--file", path,
		"--file-name", "License.pdf",
		"--description", "Updated variant",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var payload struct {
		DryRun         bool `json:"dry_run"`
		Uploads        []any
		Preserved      []any
		ProductRequest struct {
			Method string         `json:"method"`
			Path   string         `json:"path"`
			Body   map[string]any `json:"body"`
		} `json:"product_request"`
		VariantRequest struct {
			Method string         `json:"method"`
			Path   string         `json:"path"`
			Body   map[string]any `json:"body"`
		} `json:"variant_request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output is not JSON: %v", err)
	}
	if !payload.DryRun {
		t.Fatal("dry_run = false, want true")
	}
	if len(payload.Uploads) != 1 {
		t.Fatalf("uploads len = %d, want 1", len(payload.Uploads))
	}
	if len(payload.Preserved) != 1 {
		t.Fatalf("preserved len = %d, want 1", len(payload.Preserved))
	}
	if payload.ProductRequest.Method != "PUT" || payload.ProductRequest.Path != "/products/p1" {
		t.Fatalf("product request = %s %s", payload.ProductRequest.Method, payload.ProductRequest.Path)
	}
	if payload.VariantRequest.Method != "PUT" || payload.VariantRequest.Path != "/products/p1/variant_categories/vc1/variants/v1" {
		t.Fatalf("variant request = %s %s", payload.VariantRequest.Method, payload.VariantRequest.Path)
	}
	if payload.VariantRequest.Body["description"] != "Updated variant" {
		t.Fatalf("variant description = %#v, want Updated variant", payload.VariantRequest.Body["description"])
	}
	if srv.s3Calls.Load() != 0 {
		t.Fatalf("S3 calls = %d, want 0", srv.s3Calls.Load())
	}
	if srv.productPutCalls.Load() != 0 || srv.variantPutCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: product=%d variant=%d", srv.productPutCalls.Load(), srv.variantPutCalls.Load())
	}
}

func TestUpdate_FileDryRunPlainShowsRequests(t *testing.T) {
	srv := newVariantFileAttachServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeVariantUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.PlainOutput())
	cmd.SetArgs([]string{
		"v1",
		"--product", "p1",
		"--category", "vc1",
		"--file", path,
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, expected := range []string{
		"preserve\tfile_existing\tExisting.pdf",
		"upload\t" + output.EscapePlainField(path),
		"PUT\t/products/p1\t",
		"PUT\t/products/p1/variant_categories/vc1/variants/v1\t",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("plain dry-run output missing %q: %q", expected, out)
		}
	}
}

func TestUpdate_FileDryRunHumanShowsRequests(t *testing.T) {
	srv := newVariantFileAttachServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeVariantUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{
		"v1",
		"--product", "p1",
		"--category", "vc1",
		"--file", path,
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, expected := range []string{
		"Preserve existing file: Existing.pdf (file_existing)",
		"Dry run: upload " + path,
		"Dry run: PUT /products/p1",
		"Dry run: PUT /products/p1/variant_categories/vc1/variants/v1",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("human dry-run output missing %q: %q", expected, out)
		}
	}
}

func TestVariantPartialUploadErrorIncludesUploadedURLs(t *testing.T) {
	cause := errors.New("update failed")
	err := wrapVariantPartialUploadError(cause, []string{"https://example.com/file-a"})
	if !errors.Is(err, cause) {
		t.Fatalf("wrapped error does not unwrap to cause: %v", err)
	}
	if !strings.Contains(err.Error(), "previously uploaded file URLs: https://example.com/file-a") {
		t.Fatalf("error missing uploaded URLs: %v", err)
	}
}

func TestVariantPartialUploadErrorNoUploadedURLsReturnsCause(t *testing.T) {
	cause := errors.New("update failed")
	if err := wrapVariantPartialUploadError(cause, nil); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want original cause", err)
	}
	if err := wrapVariantPartialUploadError(nil, []string{"https://example.com/file-a"}); err != nil {
		t.Fatalf("nil error wrapped to %v", err)
	}
}

func TestFormatVariantExistingFileLabel(t *testing.T) {
	tests := []struct {
		name string
		file variantDryRunExistingFile
		want string
	}{
		{name: "id only", file: variantDryRunExistingFile{ID: "file_1"}, want: "file_1"},
		{name: "name equals id", file: variantDryRunExistingFile{ID: "file_1", Name: "file_1"}, want: "file_1"},
		{name: "name and id", file: variantDryRunExistingFile{ID: "file_1", Name: "License.pdf"}, want: "License.pdf (file_1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatVariantExistingFileLabel(tt.file); got != tt.want {
				t.Fatalf("label = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Delete ---

func TestDelete_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "DELETE" || gotPath != "/products/p1/variant_categories/vc1/variants/v1" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
}

func TestDelete_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newDeleteCmd()
	cmd.SetArgs([]string{"v1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required error, got: %v", err)
	}
}

func TestDelete_CategoryRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newDeleteCmd()
	cmd.SetArgs([]string{"v1", "--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--category") {
		t.Fatalf("expected --category required error, got: %v", err)
	}
}

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without confirmation")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestDelete_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"v1", "--product", "p1", "--category", "vc1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404")
	}
}
