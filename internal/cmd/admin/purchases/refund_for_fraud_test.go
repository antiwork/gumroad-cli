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

func TestRefundForFraud_RequiresEmail(t *testing.T) {
	cmd := newRefundForFraudCmd()
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundForFraud_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestRefundForFraud_SendsEmailAndShowsPurchase(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body refundForFraudRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully refunded purchase number 123 for fraud and blocked the buyer",
			"purchase": map[string]any{
				"id":             "123",
				"email":          "buyer@example.com",
				"refund_status":  "refunded",
				"purchase_state": "successful",
			},
			"subscription_cancelled": true,
		})
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/refund_for_fraud" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/refund_for_fraud", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "buyer@example.com" {
		t.Fatalf("got email %q, want buyer@example.com", body.Email)
	}
	if !strings.Contains(out, "Successfully refunded purchase number 123 for fraud and blocked the buyer") {
		t.Errorf("expected success message in output: %q", out)
	}
	if !strings.Contains(out, "Subscription: cancelled") {
		t.Errorf("expected subscription cancelled line: %q", out)
	}
}

func TestRefundForFraud_WithoutSubscriptionOmitsCancelledLine(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123 for fraud and blocked the buyer",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": false,
		})
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Contains(out, "Subscription: cancelled") {
		t.Errorf("subscription cancellation line should not appear when subscription_cancelled=false: %q", out)
	}
}

func TestRefundForFraud_DryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the refund_for_fraud endpoint")
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/refund_for_fraud") {
		t.Errorf("expected dry-run preview to mention POST and the refund_for_fraud path, got: %q", out)
	}
	if !strings.Contains(out, "email: buyer@example.com") {
		t.Errorf("expected dry-run preview to include email, got: %q", out)
	}
}

func TestRefundForFraud_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123 for fraud and blocked the buyer",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success               bool           `json:"success"`
		Message               string         `json:"message"`
		Purchase              map[string]any `json:"purchase"`
		SubscriptionCancelled bool           `json:"subscription_cancelled"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Purchase["id"] != "123" || !resp.SubscriptionCancelled {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundForFraud_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123 for fraud and blocked the buyer",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully refunded purchase number 123 for fraud and blocked the buyer\t123\tcancelled\t"
	if strings.TrimSpace(out) != strings.TrimSpace(want) {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestRefundForFraud_NoChargeSurfacesVerifyHint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Purchase has no charge to refund",
		})
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected no-charge error to surface")
	}
	if !strings.Contains(err.Error(), "Purchase has no charge to refund") {
		t.Errorf("missing underlying message: %v", err)
	}
	if !strings.Contains(err.Error(), "gumroad admin purchases view 123") {
		t.Errorf("expected verify-state hint pointing at purchase 123: %v", err)
	}
}

func TestRefundForFraud_JSONIncludesVerifyStateHint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Purchase has already been fully refunded",
		})
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error to surface")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected wrap to keep an *api.APIError on the chain so JSON classification reads the verify hint, got %T: %v", err, err)
	}
	if !strings.Contains(apiErr.Error(), "refund-for-fraud request failed:") {
		t.Errorf("APIError.Message must carry the wrap prefix for JSON output: %q", apiErr.Error())
	}
	if !strings.Contains(apiErr.Error(), "Verify status with 'gumroad admin purchases view 123'") {
		t.Errorf("APIError.Message must carry the verify-state guidance for JSON output: %q", apiErr.Error())
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status code lost across the wrap: got %d, want 422", apiErr.StatusCode)
	}
}

func TestRefundForFraud_MalformedSuccessResponseIsNotWrappedAsRequestFailed(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	})

	cmd := testutil.Command(newRefundForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected decode error to surface")
	}
	if !strings.Contains(err.Error(), "could not parse response") {
		t.Errorf("expected decode-error message: %v", err)
	}
	if strings.Contains(err.Error(), "refund-for-fraud request failed:") {
		t.Errorf("post-POST decode error must not be wrapped as a transport failure: %v", err)
	}
}
