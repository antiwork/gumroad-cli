package variants

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
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
		testutil.JSON(t, w, map[string]any{})
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
		testutil.JSON(t, w, map[string]any{})
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
		testutil.JSON(t, w, map[string]any{})
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
		testutil.JSON(t, w, map[string]any{})
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
		testutil.JSON(t, w, map[string]any{})
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
