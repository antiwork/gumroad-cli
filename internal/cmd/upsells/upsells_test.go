package upsells

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(raw) == 0 {
		return map[string]any{}
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body %q: %v", string(raw), err)
	}
	return body
}

func crossSellPayload() map[string]any {
	return map[string]any{
		"upsell": map[string]any{
			"id":                        "up1",
			"name":                      "Audiobook",
			"cross_sell":                true,
			"replace_selected_products": false,
			"universal":                 false,
			"text":                      "Add the audiobook",
			"description":               "Listen anywhere",
			"paused":                    false,
			"product": map[string]any{
				"id":            "prod-offered",
				"name":          "Audiobook",
				"currency_type": "usd",
				"variant":       nil,
			},
			"discount":          map[string]any{"type": "fixed", "cents": 500},
			"selected_products": []map[string]any{{"id": "prod-book", "name": "Book"}},
			"upsell_variants":   []map[string]any{},
		},
	}
}

func TestList_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"upsells": []map[string]any{
				{"id": "up1", "name": "Upgrade", "cross_sell": false, "paused": false, "product": map[string]any{"id": "p1", "name": "Course"}},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/upsells" {
		t.Errorf("got %s %s, want GET /upsells", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Upgrade") {
		t.Errorf("missing upsell name: %q", out)
	}
}

func TestList_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"upsells": []map[string]any{
				{"id": "up1", "name": "Upgrade", "cross_sell": false, "paused": true, "product": map[string]any{"id": "p1", "name": "Course"}},
				{"id": "up2", "name": "Audiobook", "cross_sell": true, "paused": false, "product": map[string]any{"id": "p2", "name": "Audiobook"}, "discount": map[string]any{"type": "percent", "percents": 50}},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"upsell", "cross-sell", "50% off", "yes", "no"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q: %q", want, out)
		}
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"upsells": []map[string]any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No upsells found") {
		t.Errorf("expected empty message: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"upsells": []map[string]any{{"id": "up1", "name": "Upgrade", "product": map[string]any{"id": "p1", "name": "Course"}}}})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"upsells": []map[string]any{
				{"id": "up1", "name": "Upgrade", "cross_sell": false, "paused": false, "product": map[string]any{"id": "p1", "name": "Course"}},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "up1\tUpgrade\tupsell\tCourse\tnone\tno") {
		t.Errorf("plain row mismatch: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestView_RendersCrossSell(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, crossSellPayload())
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"up1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/upsells/up1" {
		t.Errorf("got path %q", gotPath)
	}
	for _, want := range []string{"Audiobook", "cross-sell", "500 cents off", "Book (prod-book)"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q: %q", want, out)
		}
	}
}

func TestView_RendersVersionUpgrades(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"upsell": map[string]any{
				"id": "up1", "name": "Upgrade", "cross_sell": false, "paused": false,
				"product":         map[string]any{"id": "p1", "name": "Course"},
				"upsell_variants": []map[string]any{{"id": "uv1", "selected_variant": map[string]any{"id": "v1", "name": "Basic"}, "offered_variant": map[string]any{"id": "v2", "name": "Premium"}}},
			},
		})
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"up1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Basic -> Premium") {
		t.Errorf("missing version upgrade: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, crossSellPayload())
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"up1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestView_ArgRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error without upsell id")
	}
}

func TestCreate_NameRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected --name required, got: %v", err)
	}
}

func TestCreate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestCreate_VersionUpsell(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body = decodeBody(t, r)
		testutil.JSON(t, w, map[string]any{"upsell": map[string]any{"id": "up1", "name": "Upgrade", "product": map[string]any{"id": "p1", "name": "Course"}}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "Upgrade", "--product", "p1", "--offer-variant", "v1:v2"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/upsells" {
		t.Errorf("got %s %s, want POST /upsells", gotMethod, gotPath)
	}
	if body["cross_sell"] != false {
		t.Errorf("cross_sell should be false, got %v", body["cross_sell"])
	}
	variants, ok := body["upsell_variants"].([]any)
	if !ok || len(variants) != 1 {
		t.Fatalf("expected one upsell variant, got %v", body["upsell_variants"])
	}
	first := variants[0].(map[string]any)
	if first["selected_variant_id"] != "v1" || first["offered_variant_id"] != "v2" {
		t.Errorf("variant pair mismatch: %v", first)
	}
}

func TestCreate_CrossSellWithDiscountAndSelectedProducts(t *testing.T) {
	var body map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		testutil.JSON(t, w, crossSellPayload())
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "Audiobook", "--product", "prod-offered", "--cross-sell", "--selected-product", "prod-book", "--amount", "5"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body["cross_sell"] != true {
		t.Errorf("cross_sell should be true, got %v", body["cross_sell"])
	}
	ids, ok := body["product_ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "prod-book" {
		t.Errorf("product_ids mismatch: %v", body["product_ids"])
	}
	offer, ok := body["offer_code"].(map[string]any)
	if !ok || offer["amount_cents"] != float64(500) {
		t.Errorf("offer_code amount_cents mismatch: %v", body["offer_code"])
	}
}

func TestCreate_PercentOff(t *testing.T) {
	var body map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		testutil.JSON(t, w, crossSellPayload())
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "Audiobook", "--product", "p2", "--cross-sell", "--universal", "--percent-off", "50"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body["universal"] != true {
		t.Errorf("universal should be true, got %v", body["universal"])
	}
	offer, ok := body["offer_code"].(map[string]any)
	if !ok || offer["amount_percentage"] != float64(50) {
		t.Errorf("offer_code amount_percentage mismatch: %v", body["offer_code"])
	}
}

func TestCreate_AmountPercentConflict(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--amount", "5", "--percent-off", "10"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}
}

func TestCreate_PercentOutOfRange(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--percent-off", "150"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "between 1 and 100") {
		t.Fatalf("expected percent range error, got: %v", err)
	}
}

func TestCreate_OfferVariantBadFormat(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--offer-variant", "v1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "offer-variant") {
		t.Fatalf("expected offer-variant format error, got: %v", err)
	}
}

func TestCreate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"upsell": map[string]any{"id": "up1"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--name", "X", "--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestCreate_DryRun(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API in dry-run")
	})

	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--cross-sell"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Dry run") || !strings.Contains(out, "POST /upsells") {
		t.Errorf("expected dry-run output, got: %q", out)
	}
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"up1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one field to update must be provided") {
		t.Fatalf("expected no-op update error, got: %v", err)
	}
}

func TestUpdate_FetchesThenMerges(t *testing.T) {
	var putBody map[string]any
	var sawGet, sawPut bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sawGet = true
			testutil.JSON(t, w, crossSellPayload())
		case http.MethodPut:
			sawPut = true
			putBody = decodeBody(t, r)
			testutil.JSON(t, w, crossSellPayload())
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"up1", "--name", "Renamed"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !sawGet || !sawPut {
		t.Fatalf("expected GET then PUT, got get=%v put=%v", sawGet, sawPut)
	}
	if putBody["name"] != "Renamed" {
		t.Errorf("name override missing: %v", putBody["name"])
	}
	if putBody["product_id"] != "prod-offered" {
		t.Errorf("merged product_id missing: %v", putBody["product_id"])
	}
	if putBody["cross_sell"] != true {
		t.Errorf("merged cross_sell missing: %v", putBody["cross_sell"])
	}
	offer, ok := putBody["offer_code"].(map[string]any)
	if !ok || offer["amount_cents"] != float64(500) {
		t.Errorf("merged offer_code missing: %v", putBody["offer_code"])
	}
}

func TestUpdate_RemoveOffer(t *testing.T) {
	var putBody map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.JSON(t, w, crossSellPayload())
			return
		}
		putBody = decodeBody(t, r)
		testutil.JSON(t, w, crossSellPayload())
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"up1", "--remove-offer"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	offer, ok := putBody["offer_code"].(map[string]any)
	if !ok || len(offer) != 0 {
		t.Errorf("offer_code should be an explicit empty object to clear, got: %v", putBody["offer_code"])
	}
}

func TestUpdate_RemoveOfferConflict(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"up1", "--remove-offer", "--amount", "5"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--remove-offer cannot be used with") {
		t.Fatalf("expected remove-offer conflict error, got: %v", err)
	}
}

func TestUpdate_DryRun(t *testing.T) {
	var sawPut bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			sawPut = true
		}
		testutil.JSON(t, w, crossSellPayload())
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"up1", "--name", "Renamed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if sawPut {
		t.Error("PUT should not be sent in dry-run")
	}
	if !strings.Contains(out, "Dry run") || !strings.Contains(out, "PUT /upsells/up1") {
		t.Errorf("expected dry-run output, got: %q", out)
	}
}

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"up1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without confirmation")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestDelete_WithYes(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{"success": true, "message": "The upsell was deleted successfully."})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"up1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodDelete || gotPath != "/upsells/up1" {
		t.Errorf("got %s %s, want DELETE /upsells/up1", gotMethod, gotPath)
	}
}

func TestDelete_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"up1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for 404")
	}
}

func crossSellWithVariantPayload() map[string]any {
	return map[string]any{
		"upsell": map[string]any{
			"id":                        "up1",
			"name":                      "Companion",
			"cross_sell":                true,
			"replace_selected_products": false,
			"universal":                 false,
			"text":                      "Add it",
			"description":               "Great deal",
			"paused":                    false,
			"product": map[string]any{
				"id":            "prod-offered",
				"name":          "Audiobook",
				"currency_type": "usd",
				"variant":       map[string]any{"id": "var-old", "name": "Deluxe"},
			},
			"discount":          map[string]any{"type": "percent", "percents": 30},
			"selected_products": []map[string]any{{"id": "prod-book", "name": "Book"}},
			"upsell_variants":   []map[string]any{},
		},
	}
}

func versionUpsellPayload() map[string]any {
	return map[string]any{
		"upsell": map[string]any{
			"id": "up1", "name": "Pro upgrade", "cross_sell": false, "paused": false,
			"product":         map[string]any{"id": "p1", "name": "Course"},
			"upsell_variants": []map[string]any{{"id": "uv1", "selected_variant": map[string]any{"id": "v1", "name": "Basic"}, "offered_variant": map[string]any{"id": "v2", "name": "Premium"}}},
		},
	}
}

func updatePutBody(t *testing.T, args []string, getPayload map[string]any) map[string]any {
	t.Helper()
	var putBody map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.JSON(t, w, getPayload)
			return
		}
		putBody = decodeBody(t, r)
		testutil.JSON(t, w, crossSellPayload())
	})
	cmd := newUpdateCmd()
	cmd.SetArgs(args)
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	return putBody
}

func TestUpdate_ProductChangeDropsStaleVariant(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--product", "new-prod"}, crossSellWithVariantPayload())
	if body["product_id"] != "new-prod" {
		t.Errorf("product_id not applied: %v", body["product_id"])
	}
	if body["variant_id"] != "" {
		t.Errorf("stale variant_id should be cleared when product changes, got %v", body["variant_id"])
	}
}

func TestUpdate_KeepsVariantWhenProductUnchanged(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--name", "X"}, crossSellWithVariantPayload())
	if body["variant_id"] != "var-old" {
		t.Errorf("variant_id should be preserved, got %v", body["variant_id"])
	}
}

func TestUpdate_ClearSelectedProducts(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--selected-product", ""}, crossSellWithVariantPayload())
	ids, ok := body["product_ids"].([]any)
	if !ok || len(ids) != 0 {
		t.Errorf("product_ids should be cleared to empty, got %v", body["product_ids"])
	}
}

func TestUpdate_ReplaceSelectedProducts(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--selected-product", "new-book"}, crossSellWithVariantPayload())
	ids, ok := body["product_ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "new-book" {
		t.Errorf("product_ids should be replaced, got %v", body["product_ids"])
	}
}

func TestUpdate_UniversalDropsSelectedProducts(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--universal"}, crossSellWithVariantPayload())
	if body["universal"] != true {
		t.Errorf("universal should be true, got %v", body["universal"])
	}
	ids, ok := body["product_ids"].([]any)
	if !ok || len(ids) != 0 {
		t.Errorf("universal cross-sell should clear product_ids to empty, got %v", body["product_ids"])
	}
}

func TestUpdate_UniversalAndSelectedProductConflict(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Error("should not PUT on a contradictory update")
		}
		testutil.JSON(t, w, crossSellPayload())
	})
	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"up1", "--universal", "--selected-product", "x"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--universal and --selected-product cannot be used together") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestUpdate_SelectedProductOnVersionUpsellRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Error("should not PUT on a contradictory update")
		}
		testutil.JSON(t, w, versionUpsellPayload())
	})
	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"up1", "--selected-product", "x"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "pass --cross-sell") {
		t.Fatalf("expected cross-sell requirement error, got: %v", err)
	}
}

func TestUpdate_SelectedProductMakesTargeted(t *testing.T) {
	payload := crossSellWithVariantPayload()
	payload["upsell"].(map[string]any)["universal"] = true
	payload["upsell"].(map[string]any)["selected_products"] = []map[string]any{}

	body := updatePutBody(t, []string{"up1", "--selected-product", "prod1"}, payload)
	if body["universal"] != false {
		t.Errorf("targeting with --selected-product should set universal false, got %v", body["universal"])
	}
	ids, ok := body["product_ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "prod1" {
		t.Errorf("product_ids should be the targeted set, got %v", body["product_ids"])
	}
}

func TestUpdate_CrossSellClearsUpsellVariants(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--cross-sell"}, versionUpsellPayload())
	variants, ok := body["upsell_variants"].([]any)
	if !ok || len(variants) != 0 {
		t.Errorf("converting to a cross-sell should clear upsell_variants, got %v", body["upsell_variants"])
	}
}

func TestUpdate_ReplaceOfferVariants(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--offer-variant", "v3:v4"}, versionUpsellPayload())
	variants, ok := body["upsell_variants"].([]any)
	if !ok || len(variants) != 1 {
		t.Fatalf("expected one upsell variant, got %v", body["upsell_variants"])
	}
	first := variants[0].(map[string]any)
	if first["selected_variant_id"] != "v3" || first["offered_variant_id"] != "v4" {
		t.Errorf("upsell variant not replaced: %v", first)
	}
}

func TestUpdate_ConversionToVersionUpsellClearsCrossSellFields(t *testing.T) {
	body := updatePutBody(t, []string{"up1", "--cross-sell=false", "--offer-variant", "v1:v2"}, crossSellWithVariantPayload())
	if body["variant_id"] != "" {
		t.Errorf("variant_id should be cleared, got %v", body["variant_id"])
	}
	if ids, ok := body["product_ids"].([]any); !ok || len(ids) != 0 {
		t.Errorf("product_ids should be cleared, got %v", body["product_ids"])
	}
	if body["universal"] != false {
		t.Errorf("universal should be false, got %v", body["universal"])
	}
	if vs, ok := body["upsell_variants"].([]any); !ok || len(vs) != 1 {
		t.Errorf("upsell_variants should be set, got %v", body["upsell_variants"])
	}
}

func TestUpdate_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, crossSellPayload())
	})
	cmd := testutil.Command(newUpdateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"up1", "--name", "Renamed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "up1\tAudiobook") {
		t.Errorf("plain update row mismatch: %q", out)
	}
}

func TestUpdate_DryRunJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Error("PUT should not be sent in dry-run")
		}
		testutil.JSON(t, w, crossSellPayload())
	})
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"up1", "--name", "Renamed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run not valid JSON: %v\n%s", err, out)
	}
	if payload["method"] != "PUT" || payload["dry_run"] != true {
		t.Errorf("unexpected dry-run payload: %v", payload)
	}
}

func TestCreate_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"upsell": map[string]any{"id": "up9", "name": "Add-on"}})
	})
	cmd := testutil.Command(newCreateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--name", "Add-on", "--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "up9\tAdd-on") {
		t.Errorf("plain create row mismatch: %q", out)
	}
}

func TestCreate_UniversalConflictsSelectedProduct(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--cross-sell", "--universal", "--selected-product", "x"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected universal conflict error, got: %v", err)
	}
}

func TestCreate_OfferVariantOnCrossSellRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--cross-sell", "--offer-variant", "a:b"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "offer-variant applies to version upsells") {
		t.Fatalf("expected offer-variant rejection, got: %v", err)
	}
}

func TestCreate_SelectedProductRequiresCrossSell(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--selected-product", "x"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "pass --cross-sell") {
		t.Fatalf("expected cross-sell requirement error, got: %v", err)
	}
}

func TestCreate_AmountDiscount(t *testing.T) {
	var body map[string]any
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		testutil.JSON(t, w, crossSellPayload())
	})
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--product", "p1", "--cross-sell", "--amount", "5"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	offer, ok := body["offer_code"].(map[string]any)
	if !ok || offer["amount_cents"] != float64(500) {
		t.Errorf("offer_code amount_cents mismatch: %v", body["offer_code"])
	}
}

func TestCreate_DryRunJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API in dry-run")
	})
	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--name", "X", "--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run not valid JSON: %v\n%s", err, out)
	}
	if payload["method"] != "POST" {
		t.Errorf("unexpected dry-run method: %v", payload["method"])
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, crossSellWithVariantPayload())
	})
	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"up1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "cross-sell") || !strings.Contains(out, "30% off") {
		t.Errorf("plain view mismatch: %q", out)
	}
}

func TestView_FullDetail(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, crossSellWithVariantPayload())
	})
	cmd := newViewCmd()
	cmd.SetArgs([]string{"up1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	for _, want := range []string{"Offered version: Deluxe", "Text: Add it", "Description: Great deal", "30% off", "Book (prod-book)"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail view missing %q: %q", want, out)
		}
	}
}

func TestNewUpsellsCmd_RegistersSubcommands(t *testing.T) {
	cmd := NewUpsellsCmd()
	for _, args := range [][]string{{"list"}, {"view"}, {"create"}, {"update"}, {"delete"}} {
		found, _, err := cmd.Find(args)
		if err != nil {
			t.Fatalf("Find(%v) failed: %v", args, err)
		}
		if found == nil || found.Name() != args[0] {
			t.Fatalf("Find(%v) returned %v", args, found)
		}
	}
}
