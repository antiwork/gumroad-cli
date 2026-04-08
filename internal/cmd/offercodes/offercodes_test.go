package offercodes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func offerCodesHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"offer_codes": []map[string]any{
				{"id": "oc1", "name": "SAVE10", "percent_off": 10, "amount_off": 0, "max_purchase_count": 0, "universal": false},
				{"id": "oc2", "name": "FLAT5", "percent_off": 0, "amount_off": 500, "max_purchase_count": 100, "universal": true},
			},
		})
	}
}

func offerCodeHandler(t *testing.T, amountOff int, percentOff int, universal bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"offer_code": map[string]any{
				"id": "oc1", "name": "SAVE10", "amount_off": amountOff, "percent_off": percentOff,
				"max_purchase_count": 50, "universal": universal,
			},
		})
	}
}

// --- List ---

func TestList_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without --product")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"offer_codes": []map[string]any{
				{"id": "oc1", "name": "SAVE10", "percent_off": 10, "amount_off": 0},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotPath != "/products/p1/offer_codes" {
		t.Errorf("got path %q", gotPath)
	}
	if !strings.Contains(out, "SAVE10") {
		t.Errorf("missing offer code name: %q", out)
	}
}

func TestList_Table(t *testing.T) {
	testutil.Setup(t, offerCodesHandler(t))

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "10%") {
		t.Errorf("missing percent discount: %q", out)
	}
	if !strings.Contains(out, "500 cents off") {
		t.Errorf("missing amount discount: %q", out)
	}
	if !strings.Contains(out, "100") {
		t.Errorf("missing max uses: %q", out)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("missing universal yes: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, offerCodesHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, offerCodesHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "oc1") || !strings.Contains(out, "SAVE10") {
		t.Errorf("plain missing data: %q", out)
	}
	if !strings.Contains(out, "oc1\tSAVE10\t10%\tunlimited\tno") {
		t.Errorf("plain missing unlimited/non-universal columns: %q", out)
	}
	if !strings.Contains(out, "oc2\tFLAT5\t500 cents off\t100\tyes") {
		t.Errorf("plain missing max uses/universal columns: %q", out)
	}
}

func TestList_NoColorDisablesANSI(t *testing.T) {
	testutil.Setup(t, offerCodesHandler(t))
	testutil.SetStdoutIsTerminal(t, true)

	cmd := testutil.Command(newListCmd(), testutil.NoColor(true))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	testutil.AssertNoANSI(t, out)
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"offer_codes": []map[string]any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "No offer codes found") {
		t.Errorf("expected empty message: %q", out)
	}
}

func TestList_IntegerLikeFloats(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"offer_codes": [{
				"id": "oc1",
				"name": "SAVE10",
				"amount_off": 500.0,
				"percent_off": 0.0,
				"max_purchase_count": 25.0,
				"universal": true
			}]
		}`)
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "500 cents off") || !strings.Contains(out, "25") {
		t.Fatalf("output missing float-backed fields: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- View ---

func TestView_PercentOff(t *testing.T) {
	testutil.Setup(t, offerCodeHandler(t, 0, 20, false))

	cmd := newViewCmd()
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "SAVE10") {
		t.Errorf("missing name: %q", out)
	}
	if !strings.Contains(out, "20%") {
		t.Errorf("missing percent discount: %q", out)
	}
}

func TestView_AmountOff_Universal(t *testing.T) {
	testutil.Setup(t, offerCodeHandler(t, 500, 0, true))

	cmd := newViewCmd()
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "500 cents") {
		t.Errorf("missing amount discount: %q", out)
	}
	if !strings.Contains(out, "Universal: yes") {
		t.Errorf("missing universal: %q", out)
	}
	if !strings.Contains(out, "Max uses: 50") {
		t.Errorf("missing max uses: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, offerCodeHandler(t, 0, 10, false))

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestView_Plain_PercentOff(t *testing.T) {
	testutil.Setup(t, offerCodeHandler(t, 0, 15, false))

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "15%") {
		t.Errorf("plain missing percent: %q", out)
	}
}

func TestView_Plain_AmountOff(t *testing.T) {
	testutil.Setup(t, offerCodeHandler(t, 300, 0, false))

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "300 cents") {
		t.Errorf("plain missing amount: %q", out)
	}
}

func TestView_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"oc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

// --- Create ---

func TestCreate_MutualExclusion(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--amount", "5", "--percent-off", "10"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("expected usage help in %q", err.Error())
	}
}

func TestCreate_RequiresDiscount(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without discount type")
	}
}


func TestCreate_RejectsNegativePercentOff(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--percent-off", "-10"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--percent-off must be between 1 and 100") {
		t.Fatalf("expected percent validation error, got: %v", err)
	}
}

func TestCreate_RejectsPercentOffAbove100(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--percent-off", "101"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--percent-off must be between 1 and 100") {
		t.Fatalf("expected percent validation error, got: %v", err)
	}
}

func TestCreate_RejectsNegativeMaxPurchaseCount(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--percent-off", "10", "--max-purchase-count", "-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-purchase-count cannot be negative") {
		t.Fatalf("expected max purchase validation error, got: %v", err)
	}
}

func TestCreate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "X", "--percent-off", "10"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestCreate_NameRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--percent-off", "10"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected --name required, got: %v", err)
	}
}

func TestCreate_PercentOff(t *testing.T) {
	var gotPercentOff, gotAmountOff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPercentOff = r.PostForm.Get("percent_off")
		gotAmountOff = r.PostForm.Get("amount_off")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "SAVE20", "--percent-off", "20"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotPercentOff != "20" {
		t.Errorf("got percent_off=%q, want 20", gotPercentOff)
	}
	if gotAmountOff != "" {
		t.Errorf("amount_off should be empty, got %q", gotAmountOff)
	}
}

func TestCreate_Amount(t *testing.T) {
	var gotAmountOff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotAmountOff = r.PostForm.Get("amount_off")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "FLAT5", "--amount", "5.00"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotAmountOff != "500" {
		t.Errorf("got amount_off=%q, want 500", gotAmountOff)
	}
}

func TestCreate_AmountWholeNumber(t *testing.T) {
	var gotAmountOff string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotAmountOff = r.PostForm.Get("amount_off")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "FLAT5", "--amount", "5"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotAmountOff != "500" {
		t.Errorf("got amount_off=%q, want 500", gotAmountOff)
	}
}

func TestCreate_AmountAndPercentOffConflict(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--amount", "5", "--percent-off", "10"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}
}

func TestCreate_AmountInvalidInput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--amount", "abc"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a valid amount") {
		t.Fatalf("expected validation error, got: %v", err)
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *cmdutil.UsageError, got %T", err)
	}
}

func TestCreate_AmountZeroRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--amount", "0"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--amount must be greater than 0") {
		t.Fatalf("expected amount validation error, got: %v", err)
	}
}

func TestCreate_Universal(t *testing.T) {
	var gotUniversal, gotMaxPurchase string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotUniversal = r.PostForm.Get("universal")
		gotMaxPurchase = r.PostForm.Get("max_purchase_count")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "UNI", "--percent-off", "5", "--universal", "--max-purchase-count", "100"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotUniversal != "true" {
		t.Errorf("got universal=%q, want true", gotUniversal)
	}
	if gotMaxPurchase != "100" {
		t.Errorf("got max_purchase_count=%q, want 100", gotMaxPurchase)
	}
}

func TestCreate_MaxPurchaseCountZero(t *testing.T) {
	var gotMaxPurchase string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotMaxPurchase = r.PostForm.Get("max_purchase_count")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "ZERO", "--percent-off", "5", "--max-purchase-count", "0"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMaxPurchase != "0" {
		t.Errorf("got max_purchase_count=%q, want 0", gotMaxPurchase)
	}
}

func TestCreate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"offer_code": map[string]any{"id": "oc1"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--name", "X", "--percent-off", "10"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

// --- Update ---

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
	cmd.SetArgs([]string{"oc1", "--product", "p1", "--max-purchase-count", "100"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotParam != "100" {
		t.Errorf("got max_purchase_count=%q, want 100", gotParam)
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
	cmd.SetArgs([]string{"oc1", "--product", "p1", "--max-purchase-count", "0"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotParam != "0" {
		t.Errorf("got max_purchase_count=%q, want 0", gotParam)
	}
}

func TestUpdate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"oc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
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
	cmd.SetArgs([]string{"oc1", "--product", "p1", "--max-purchase-count", "-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-purchase-count cannot be negative") {
		t.Fatalf("expected max purchase validation error, got: %v", err)
	}
}

func TestUpdate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"offer_code": map[string]any{"id": "oc1"}})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"oc1", "--product", "p1", "--max-purchase-count", "50"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

// --- Delete ---

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
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
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "DELETE" || gotPath != "/products/p1/offer_codes/oc1" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
}

func TestDelete_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newDeleteCmd()
	cmd.SetArgs([]string{"oc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestDelete_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"oc1", "--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404")
	}
}
