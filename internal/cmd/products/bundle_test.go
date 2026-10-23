package products

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func bundleResponse(items ...map[string]any) map[string]any {
	if items == nil {
		items = []map[string]any{}
	}
	return map[string]any{
		"success": true,
		"product": map[string]any{"id": "bun_1", "name": "Starter Bundle", "bundle_products": items},
	}
}

func TestBundleSet_SendsProductsInOrder(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Products []bundleItem `json:"products"`
	}
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("body is not JSON: %v: %s", err, raw)
		}
		testutil.JSON(t, w, bundleResponse(
			map[string]any{"product_id": "p_a", "variant_id": "v_1", "quantity": 2, "position": 0},
			map[string]any{"product_id": "p_b", "variant_id": nil, "quantity": 1, "position": 1},
		))
	})

	cmd := testutil.Command(newBundleSetCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"bun_1", "--product", "p_a", "--product", "p_b", "--variant", "p_a=v_1", "--quantity", "p_a=2"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != "/products/bun_1/bundle_contents" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	want := []bundleItem{
		{ProductID: "p_a", VariantID: "v_1", Quantity: 2, Position: 0},
		{ProductID: "p_b", Quantity: 1, Position: 1},
	}
	if len(gotBody.Products) != 2 || gotBody.Products[0] != want[0] || gotBody.Products[1] != want[1] {
		t.Fatalf("body products = %+v, want %+v", gotBody.Products, want)
	}
	for _, s := range []string{"now contains 2 product(s)", "p_a", "v_1", "p_b"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q in %q", s, out)
		}
	}
}

func TestBundleSet_PaddedProductIDs(t *testing.T) {
	const paddedProduct = "-cGksPcArAUU8j_XTYsrnQ=="
	const paddedVariant = "vGksPcArAUU8j_XTYsrnQ=="
	var gotBody struct {
		Products []bundleItem `json:"products"`
	}
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("body is not JSON: %v: %s", err, raw)
		}
		testutil.JSON(t, w, bundleResponse(map[string]any{"product_id": paddedProduct, "variant_id": paddedVariant, "quantity": 2, "position": 0}))
	})

	cmd := testutil.Command(newBundleSetCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{
		"bun_1",
		"--product", paddedProduct,
		"--variant", paddedProduct + "=" + paddedVariant,
		"--quantity", paddedProduct + "=2",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if len(gotBody.Products) != 1 {
		t.Fatalf("products = %+v", gotBody.Products)
	}
	if got := gotBody.Products[0]; got.ProductID != paddedProduct || got.VariantID != paddedVariant || got.Quantity != 2 {
		t.Fatalf("item = %+v", got)
	}
}

func TestSplitBundlePair(t *testing.T) {
	index := map[string]int{"p_a": 0, "p_a==": 1, "-cGksPcArAUU8j_XTYsrnQ==": 2}
	cases := []struct {
		name, raw, wantID, wantValue string
		wantErr                      bool
	}{
		{"plain", "p_a=v_1", "p_a", "v_1", false},
		{"padded id", "p_a===v_1", "p_a==", "v_1", false},
		{"real padded id", "-cGksPcArAUU8j_XTYsrnQ===2", "-cGksPcArAUU8j_XTYsrnQ==", "2", false},
		{"no separator", "p_a", "", "", true},
		{"empty value", "p_a=", "", "", true},
		{"unknown id", "p_z=v_1", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, value, err := splitBundlePair("--variant", tc.raw, index)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q=%q", id, value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID || value != tc.wantValue {
				t.Fatalf("got %q=%q, want %q=%q", id, value, tc.wantID, tc.wantValue)
			}
		})
	}
}

func TestBundleSet_JSONPassesResponseThrough(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, bundleResponse(map[string]any{"product_id": "p_a", "quantity": 1, "position": 0}))
	})
	cmd := testutil.Command(newBundleSetCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"bun_1", "--product", "p_a"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp bundleProductResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not JSON: %v: %s", err, out)
	}
	if len(resp.Product.BundleProducts) != 1 || resp.Product.BundleProducts[0].ProductID != "p_a" {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestBundleSet_PlainOutput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, bundleResponse(map[string]any{"product_id": "p_a", "variant_id": "v_9", "quantity": 3, "position": 0}))
	})
	cmd := testutil.Command(newBundleSetCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"bun_1", "--product", "p_a"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "p_a\tv_9\t3" {
		t.Fatalf("plain = %q", out)
	}
}

func TestBundleSet_APIErrorSurfaces(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success":false,"message":"This product is not a bundle."}`)
	})
	cmd := testutil.Command(newBundleSetCmd())
	cmd.SetArgs([]string{"prod_1", "--product", "p_a"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a bundle") {
		t.Fatalf("err = %v", err)
	}
}

func TestBundleSet_DryRunSkipsAPI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate testutil.OptionsMutator
		want   []string
	}{
		{"json", testutil.JSONOutput(), []string{`"dry_run":true`, `"/products/bun_1/bundle_contents"`, `"product_id":"p_a"`, `"variant_id":"v_1"`}},
		{"plain", testutil.PlainOutput(), []string{"PUT\t/products/bun_1/bundle_contents\tp_a\tv_1\t1"}},
		{"human", testutil.Quiet(false), []string{"Dry run", "1. p_a x1 (variant v_1)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) { t.Error("should not reach API") })
			cmd := testutil.Command(newBundleSetCmd(), testutil.DryRun(true), tc.mutate)
			cmd.SetArgs([]string{"bun_1", "--product", "p_a", "--variant", "p_a=v_1"})
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
			compact := strings.ReplaceAll(strings.ReplaceAll(out, " ", ""), "\n", "")
			for _, w := range tc.want {
				if !strings.Contains(out, w) && !strings.Contains(compact, strings.ReplaceAll(w, " ", "")) {
					t.Fatalf("missing %q in %q", w, out)
				}
			}
		})
	}
}

func TestBundleSet_UsageErrors(t *testing.T) {
	cases := map[string][]string{
		"no product":          {"bun_1"},
		"empty product":       {"bun_1", "--product", " "},
		"duplicate product":   {"bun_1", "--product", "p_a", "--product", "p_a"},
		"variant bad format":  {"bun_1", "--product", "p_a", "--variant", "p_a"},
		"variant unknown id":  {"bun_1", "--product", "p_a", "--variant", "p_z=v_1"},
		"quantity not number": {"bun_1", "--product", "p_a", "--quantity", "p_a=two"},
		"quantity zero":       {"bun_1", "--product", "p_a", "--quantity", "p_a=0"},
		"no bundle id":        {"--product", "p_a"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) { t.Error("should not reach API") })
			cmd := testutil.Command(newBundleSetCmd())
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestBundleList_RendersItems(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/products/bun_1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, bundleResponse(map[string]any{"product_id": "p_a", "quantity": 1, "position": 0}))
	})
	cmd := testutil.Command(newBundleCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"list", "bun_1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "p_a") || !strings.Contains(out, "PRODUCT") {
		t.Fatalf("out = %q", out)
	}
}

func TestBundleList_EmptyHintsSet(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, bundleResponse())
	})
	cmd := testutil.Command(newBundleListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"bun_1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "gumroad products bundle set bun_1") {
		t.Fatalf("out = %q", out)
	}
}

func TestBundleList_QuietPrintsNothing(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, bundleResponse(map[string]any{"product_id": "p_a", "quantity": 1, "position": 0}))
	})
	cmd := testutil.Command(newBundleListCmd())
	cmd.SetArgs([]string{"bun_1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "" {
		t.Fatalf("quiet out = %q", out)
	}
}

func TestCreateBundle_HintsBundleSet(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "product": map[string]any{"id": "bun_1", "name": "Starter Bundle", "published": false}})
	})
	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Starter Bundle", "--type", "bundle", "--price", "29"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "gumroad products bundle set bun_1 --product <product_id>") {
		t.Fatalf("out = %q", out)
	}
}
