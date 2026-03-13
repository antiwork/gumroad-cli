package skus

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_ProductIDRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without product ID")
	})

	cmd := NewProductSKUsCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without product ID")
	}
	if !strings.Contains(err.Error(), "missing required argument: <id>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"skus": []map[string]any{
				{"id": "sku1", "name": "Small+Red", "price_difference_cents": 0, "max_purchase_count": 10},
			},
		})
	})

	cmd := NewProductSKUsCmd()
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotPath != "/products/p1/skus" {
		t.Errorf("got path %q, want /products/p1/skus", gotPath)
	}
	if !strings.Contains(out, "Small+Red") {
		t.Errorf("output missing SKU name: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"skus": []map[string]any{
				{"id": "sku1", "name": "S"},
			},
		})
	})

	cmd := testutil.Command(NewProductSKUsCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"skus": []any{}})
	})

	cmd := testutil.Command(NewProductSKUsCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "No SKUs found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"skus": []map[string]any{
				{"id": "sku1", "name": "Small", "price_difference_cents": 100, "max_purchase_count": 0},
			},
		})
	})

	cmd := testutil.Command(NewProductSKUsCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "sku1") {
		t.Errorf("plain output missing data: %q", out)
	}
	if !strings.Contains(out, "sku1\tSmall\t100\tunlimited") {
		t.Errorf("plain output missing max purchases column: %q", out)
	}
}

func TestList_NoColorDisablesANSI(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"skus": []map[string]any{
				{"id": "sku1", "name": "Small", "price_difference_cents": 100, "max_purchase_count": 0},
			},
		})
	})
	testutil.SetStdoutIsTerminal(t, true)

	cmd := testutil.Command(NewProductSKUsCmd(), testutil.NoColor(true))
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	testutil.AssertNoANSI(t, out)
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := NewProductSKUsCmd()
	cmd.SetArgs([]string{"p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_UnlimitedPurchases(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"skus": []map[string]any{
				{"id": "sku1", "name": "Basic", "price_difference_cents": 0, "max_purchase_count": 0},
			},
		})
	})

	cmd := NewProductSKUsCmd()
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "unlimited") {
		t.Errorf("max_purchase_count=0 should show unlimited: %q", out)
	}
}

func TestList_IntegerLikeFloats(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"skus": [{
				"id": "sku1",
				"name": "Small+Red",
				"price_difference_cents": 100.0,
				"max_purchase_count": 5.0
			}]
		}`)
	})

	cmd := NewProductSKUsCmd()
	cmd.SetArgs([]string{"p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "100") || !strings.Contains(out, "5") {
		t.Fatalf("output missing float-backed fields: %q", out)
	}
}
