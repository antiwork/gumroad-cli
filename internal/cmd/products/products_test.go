package products

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/products" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{
				{"id": "p1", "name": "Art Pack", "published": true, "formatted_price": "$10", "sales_count": 42},
				{"id": "p2", "name": "E-Book", "published": false, "formatted_price": "$25", "sales_count": 0},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &resp); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jsonErr, out)
	}
	products := resp["products"].([]any)
	if len(products) != 2 {
		t.Errorf("got %d products, want 2", len(products))
	}
}

func TestList_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{
				{"id": "p1", "name": "Art Pack", "published": true, "formatted_price": "$10", "sales_count": 42},
			},
		})
	})

	cmd := newListCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "p1") || !strings.Contains(out, "Art Pack") {
		t.Errorf("table output missing product data: %q", out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("table output missing sales count: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{
				{"id": "p1", "name": "Art", "published": true, "formatted_price": "$10", "sales_count": 5},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "p1\t") {
		t.Errorf("plain output missing tab-separated data: %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"products": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "No products found") {
		t.Errorf("expected empty state message, got: %q", out)
	}
}

func TestView_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "prod123", "name": "Test", "published": true,
				"formatted_price": "$5", "sales_count": 10, "sales_usd_cents": 5000,
				"short_url": "https://gum.co/test",
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"prod123"}) })
	if gotPath != "/products/prod123" {
		t.Errorf("got path %q, want /products/prod123", gotPath)
	}
	if !strings.Contains(out, "Test") {
		t.Errorf("output missing product name: %q", out)
	}
	if !strings.Contains(out, "$50.00") {
		t.Errorf("output missing revenue calculation: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "p1", "name": "X"},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestView_SalesUSDCentsFloat(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"product": {
				"id": "p1",
				"name": "Float Revenue",
				"published": true,
				"formatted_price": "$5",
				"sales_count": 0,
				"sales_usd_cents": 0.0
			}
		}`)
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "Float Revenue") {
		t.Errorf("output missing product name: %q", out)
	}
	if !strings.Contains(out, "$0.00") {
		t.Errorf("output missing revenue calculation for float cents: %q", out)
	}
}

func TestView_SalesUSDCentsNull(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"product": {
				"id": "p1",
				"name": "Null Revenue",
				"published": true,
				"formatted_price": "$5",
				"sales_count": 0,
				"sales_usd_cents": null
			}
		}`)
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "$0.00") {
		t.Errorf("output missing revenue calculation for null cents: %q", out)
	}
}

func TestView_SalesUSDCentsMissing(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"product": {
				"id": "p1",
				"name": "Missing Revenue",
				"published": true,
				"formatted_price": "$5",
				"sales_count": 0
			}
		}`)
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "$0.00") {
		t.Errorf("output missing revenue calculation for missing cents: %q", out)
	}
}

func TestView_SalesCountFloat(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"product": {
				"id": "p1",
				"name": "Float Count",
				"published": true,
				"formatted_price": "$5",
				"sales_count": 3.0,
				"sales_usd_cents": 1500.0
			}
		}`)
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "Sales: 3 ($15.00)") {
		t.Errorf("output missing integer-like float sales count: %q", out)
	}
}

func TestList_SalesCountFloat(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"products": [{
				"id": "p1",
				"name": "Float Count",
				"published": true,
				"formatted_price": "$10",
				"sales_count": 5.0
			}]
		}`)
	})

	cmd := newListCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "Float Count") || !strings.Contains(out, "5") {
		t.Errorf("list output missing float sales count product data: %q", out)
	}
}

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	err := cmd.RunE(cmd, []string{"prod1"})
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestNewProductsCmd_HelpMentionsCreateUpdateLimitation(t *testing.T) {
	cmd := NewProductsCmd()
	if !strings.Contains(cmd.Long, "does not support creating or updating products") {
		t.Fatalf("expected products help to mention create/update limitation, got %q", cmd.Long)
	}
}

func TestDelete_WithYes(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"prod1"}) })
	if gotMethod != "DELETE" {
		t.Errorf("got method %q, want DELETE", gotMethod)
	}
	if gotPath != "/products/prod1" {
		t.Errorf("got path %q, want /products/prod1", gotPath)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected deletion confirmation, got: %q", out)
	}
}

func TestEnable_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newEnableCmd()
	testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/p1/enable" {
		t.Errorf("got path %q, want /products/p1/enable", gotPath)
	}
}

func TestDisable_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newDisableCmd()
	testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/p1/disable" {
		t.Errorf("got path %q, want /products/p1/disable", gotPath)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Test", "published": true,
				"formatted_price": "$5", "sales_count": 10, "sales_usd_cents": 5000,
				"short_url": "https://gum.co/test",
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "p1") || !strings.Contains(out, "Test") {
		t.Errorf("plain view missing data: %q", out)
	}
}

func TestEnable_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newEnableCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "enabled") {
		t.Errorf("expected enabled message, got: %q", out)
	}
}

func TestDisable_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDisableCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected disabled message, got: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestView_WithDescription(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Book", "published": true,
				"formatted_price": "$20", "sales_count": 5, "sales_usd_cents": 10000,
				"short_url": "https://gum.co/book", "description": "A great book",
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "A great book") {
		t.Errorf("missing description: %q", out)
	}
	if !strings.Contains(out, "gum.co/book") {
		t.Errorf("missing URL: %q", out)
	}
}

func TestList_DraftStatus(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{
				{"id": "p1", "name": "Draft", "published": false, "formatted_price": "$5", "sales_count": 0},
			},
		})
	})

	cmd := newListCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "draft") {
		t.Errorf("should show draft status: %q", out)
	}
}

func TestList_Tip(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{
				{"id": "p1", "name": "Art", "published": true, "formatted_price": "$10", "sales_count": 1},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "Tip") {
		t.Errorf("should show tip when not quiet: %q", out)
	}
}

func TestList_PlainDraftStatus(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{
				{"id": "p1", "name": "Draft", "published": false, "formatted_price": "$5", "sales_count": 0},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "draft") {
		t.Errorf("plain should show draft: %q", out)
	}
}

func TestView_DraftPlain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Draft", "published": false,
				"formatted_price": "$5", "sales_count": 0, "sales_usd_cents": 0,
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "draft") {
		t.Errorf("plain view should show draft: %q", out)
	}
}

func TestDelete_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	err := cmd.RunE(cmd, []string{"p1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnable_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newEnableCmd()
	err := cmd.RunE(cmd, []string{"p1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisable_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newDisableCmd()
	err := cmd.RunE(cmd, []string{"p1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewProductsCmd(t *testing.T) {
	cmd := NewProductsCmd()
	if cmd.Use != "products" {
		t.Errorf("got Use=%q, want products", cmd.Use)
	}
	subs := make(map[string]bool)
	for _, c := range cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"list", "view <id>", "delete <id>", "enable <id>", "disable <id>", "skus <id>"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestNewProductsCmd_HelpIncludesSKUs(t *testing.T) {
	cmd := NewProductsCmd()
	cmd.SetArgs([]string{"--help"})

	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute help: %v", err)
		}
	})

	for _, want := range []string{
		"gumroad products skus <id>",
		"skus        List SKUs for a product",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in help output %q", want, out)
		}
	}
}

func TestView_NoDescription(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Simple", "published": false,
				"formatted_price": "$1", "sales_count": 0, "sales_usd_cents": 0,
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "Simple") {
		t.Errorf("missing name: %q", out)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func countedImageServer(t *testing.T, data []byte, contentType string) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	}))
	return srv, &hits
}

func TestView_WithThumbnail(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, _ := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	thumbURL := imgSrv.URL + "/thumb.png"
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Art", "published": true,
				"formatted_price": "$10", "sales_count": 5, "sales_usd_cents": 5000,
				"thumbnail_url": thumbURL,
			},
		})
	})
	testutil.SetColorEnabled(t, true)

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "▄") {
		t.Errorf("expected half-block image in output: %q", out)
	}
	if !strings.Contains(out, "Art") {
		t.Errorf("expected product name in output: %q", out)
	}
}

func TestView_NoImageFlag(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, hits := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	thumbURL := imgSrv.URL + "/thumb.png"
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Art", "published": true,
				"formatted_price": "$10", "sales_count": 5, "sales_usd_cents": 5000,
				"thumbnail_url": thumbURL,
			},
		})
	})
	testutil.SetColorEnabled(t, true)

	cmd := testutil.Command(newViewCmd(), testutil.NoImage(true))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if strings.Contains(out, "▄") {
		t.Error("expected no half-block image when --no-image is set")
	}
	if hits.Load() != 0 {
		t.Errorf("expected no image fetch when --no-image is set, got %d hits", hits.Load())
	}
	if !strings.Contains(out, "Art") {
		t.Errorf("expected product name in output: %q", out)
	}
}

func TestView_NullThumbnail(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "NoThumb", "published": true,
				"formatted_price": "$5", "sales_count": 0, "sales_usd_cents": 0,
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if !strings.Contains(out, "NoThumb") {
		t.Errorf("expected product name in output: %q", out)
	}
}

func TestView_JSONSkipsImageFetch(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, hits := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "JSON", "thumbnail_url": imgSrv.URL + "/thumb.png",
			},
		})
	})
	testutil.SetColorEnabled(t, true)

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if hits.Load() != 0 {
		t.Errorf("expected no image fetch for JSON output, got %d hits", hits.Load())
	}
	if !strings.Contains(out, `"id": "p1"`) {
		t.Errorf("expected JSON output: %q", out)
	}
}

func TestView_JQSkipsImageFetch(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, hits := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "JQ", "thumbnail_url": imgSrv.URL + "/thumb.png",
			},
		})
	})
	testutil.SetColorEnabled(t, true)

	cmd := testutil.Command(newViewCmd(), testutil.JQ(".product.id"))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if hits.Load() != 0 {
		t.Errorf("expected no image fetch for jq output, got %d hits", hits.Load())
	}
	if strings.TrimSpace(out) != `"p1"` {
		t.Errorf("expected jq-filtered output, got %q", strings.TrimSpace(out))
	}
}

func TestView_PlainSkipsImageFetch(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, hits := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "Plain", "published": true,
				"formatted_price": "$5", "sales_count": 1, "sales_usd_cents": 500,
				"thumbnail_url": imgSrv.URL + "/thumb.png",
			},
		})
	})
	testutil.SetColorEnabled(t, true)

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if hits.Load() != 0 {
		t.Errorf("expected no image fetch for plain output, got %d hits", hits.Load())
	}
	if !strings.Contains(out, "p1\tPlain\tpublished\t$5\t1") {
		t.Errorf("expected plain output: %q", out)
	}
}

func TestView_ColorDisabledSkipsImageFetch(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, hits := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id": "p1", "name": "No Color", "published": true,
				"formatted_price": "$5", "sales_count": 1, "sales_usd_cents": 500,
				"thumbnail_url": imgSrv.URL + "/thumb.png",
			},
		})
	})
	testutil.SetColorEnabled(t, false)

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if hits.Load() != 0 {
		t.Errorf("expected no image fetch when color is disabled, got %d hits", hits.Load())
	}
	if !strings.Contains(out, "No Color") {
		t.Errorf("expected product output when color is disabled: %q", out)
	}
}

func TestView_UsesPreviewWhenThumbnailEmpty(t *testing.T) {
	pngData := testPNG(t)
	imgSrv, hits := countedImageServer(t, pngData, "image/png")
	defer imgSrv.Close()

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"product": {
				"id": "p1",
				"name": "Preview Fallback",
				"published": true,
				"formatted_price": "$5",
				"sales_count": 1,
				"sales_usd_cents": 500,
				"thumbnail_url": "",
				"preview_url": "`+imgSrv.URL+`/preview.webp"
			}
		}`)
	})
	testutil.SetColorEnabled(t, true)

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"p1"}) })
	if hits.Load() != 1 {
		t.Errorf("expected preview image fetch, got %d hits", hits.Load())
	}
	if !strings.Contains(out, "▄") {
		t.Errorf("expected preview image rendering in output: %q", out)
	}
}
