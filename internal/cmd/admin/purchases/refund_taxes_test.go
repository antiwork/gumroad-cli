package purchases

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRefundTaxes_RequiresEmail(t *testing.T) {
	cmd := newRefundTaxesCmd()
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundTaxes_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestRefundTaxes_OmitsOptionalFieldsWhenAbsent(t *testing.T) {
	var bodyKeys map[string]json.RawMessage
	var body refundTaxesRequest
	var gotMethod, gotPath string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.Unmarshal(raw, &bodyKeys); err != nil {
			t.Fatalf("decode body keys: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":  "Successfully refunded taxes for purchase number 123",
			"purchase": map[string]any{"id": "123", "email": "buyer@example.com"},
		})
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/refund_taxes" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/refund_taxes", gotMethod, gotPath)
	}
	if body.Email != "buyer@example.com" {
		t.Errorf("got email %q, want buyer@example.com", body.Email)
	}
	if _, present := bodyKeys["note"]; present {
		t.Errorf("note must be omitted when not set, got body keys: %v", bodyKeys)
	}
	if _, present := bodyKeys["business_vat_id"]; present {
		t.Errorf("business_vat_id must be omitted when not set, got body keys: %v", bodyKeys)
	}
	if !strings.Contains(out, "Successfully refunded taxes for purchase number 123") {
		t.Errorf("expected success message: %q", out)
	}
}

func TestRefundTaxes_ForwardsNoteAndBusinessVATID(t *testing.T) {
	var body refundTaxesRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":  "ok",
			"purchase": map[string]any{"id": "123"},
		})
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"123",
		"--email", "buyer@example.com",
		"--note", "buyer self-accounts for VAT",
		"--business-vat-id", "GB123456789",
	})
	testutil.MustExecute(t, cmd)

	if body.Note != "buyer self-accounts for VAT" {
		t.Errorf("got note %q, want forwarded value", body.Note)
	}
	if body.BusinessVATID != "GB123456789" {
		t.Errorf("got business_vat_id %q, want GB123456789", body.BusinessVATID)
	}
}

func TestRefundTaxes_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--business-vat-id", "GB123456789"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/refund_taxes") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	if !strings.Contains(out, "business_vat_id: GB123456789") {
		t.Errorf("expected business_vat_id in dry-run preview, got: %q", out)
	}
}

func TestRefundTaxes_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":  "Successfully refunded taxes for purchase number 123",
			"purchase": map[string]any{"id": "123"},
		})
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success  bool           `json:"success"`
		Purchase map[string]any `json:"purchase"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Purchase["id"] != "123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundTaxes_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":  "Successfully refunded taxes for purchase number 123",
			"purchase": map[string]any{"id": "123"},
		})
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully refunded taxes for purchase number 123\t123"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestRefundTaxes_JSONIncludesVerifyStateHint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "No refundable taxes available",
		})
	})

	cmd := testutil.Command(newRefundTaxesCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected refund-taxes error to surface")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected wrap to keep an *api.APIError on the chain, got %T: %v", err, err)
	}
	if !strings.Contains(apiErr.Error(), "refund-taxes request failed:") {
		t.Errorf("APIError.Message must carry the wrap prefix: %q", apiErr.Error())
	}
	if !strings.Contains(apiErr.Error(), "Verify status with 'gumroad admin purchases view 123'") {
		t.Errorf("APIError.Message must carry the verify-state guidance: %q", apiErr.Error())
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status code lost across the wrap: got %d, want 422", apiErr.StatusCode)
	}
}
