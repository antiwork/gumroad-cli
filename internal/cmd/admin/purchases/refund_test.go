package purchases

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRefund_RequiresEmail(t *testing.T) {
	cmd := newRefundCmd()
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefund_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestRefund_FullSendsEmailAndOmitsAmountCents(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body refundRequest
	var bodyKeys map[string]json.RawMessage

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
		if err := json.Unmarshal(raw, &bodyKeys); err != nil {
			t.Fatalf("decode body keys: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully refunded purchase number 123",
			"purchase": map[string]any{
				"id":             "123",
				"email":          "buyer@example.com",
				"refund_status":  "refunded",
				"purchase_state": "successful",
			},
			"subscription_cancelled": false,
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/refund" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/refund", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email/amount must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "buyer@example.com" {
		t.Fatalf("got email %q, want buyer@example.com", body.Email)
	}
	if _, present := bodyKeys["amount_cents"]; present {
		t.Errorf("amount_cents must be omitted on full refund, got body keys: %v", bodyKeys)
	}
	if _, present := bodyKeys["force"]; present {
		t.Errorf("force should be omitted when not set, got body keys: %v", bodyKeys)
	}
	if _, present := bodyKeys["cancel_subscription"]; present {
		t.Errorf("cancel_subscription should be omitted when not set, got body keys: %v", bodyKeys)
	}
	if !strings.Contains(out, "Successfully refunded purchase number 123") {
		t.Errorf("expected success message in output: %q", out)
	}
}

func TestRefund_PartialSendsAmountCents(t *testing.T) {
	var body refundRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": false,
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "5.00"})
	testutil.MustExecute(t, cmd)

	if body.AmountCents != 500 {
		t.Errorf("got amount_cents=%d, want 500", body.AmountCents)
	}
}

func TestRefund_RejectsInvalidAmount(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "abc"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a valid amount") {
		t.Fatalf("expected validation error, got: %v", err)
	}
}

func TestRefund_RejectsZeroAmount(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--amount must be greater than 0") {
		t.Fatalf("expected zero-amount error, got: %v", err)
	}
}

func TestRefund_ForwardsForceAndCancelSubscription(t *testing.T) {
	var body refundRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--force", "--cancel-subscription"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !body.Force {
		t.Errorf("expected force=true in body, got %#v", body)
	}
	if !body.CancelSubscription {
		t.Errorf("expected cancel_subscription=true in body, got %#v", body)
	}
	if !strings.Contains(out, "Subscription: cancelled") {
		t.Errorf("expected subscription cancelled message: %q", out)
	}
}

func TestRefund_ShowsSubscriptionCancelError(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                   "Successfully refunded purchase number 123",
			"purchase":                  map[string]any{"id": "123"},
			"subscription_cancelled":    false,
			"subscription_cancel_error": "stripe blew up",
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--cancel-subscription"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Subscription cancel failed: stripe blew up") {
		t.Errorf("expected cancel failure message: %q", out)
	}
}

func TestRefund_DryRunSkipsConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		// Admin commands today do not short-circuit on dry-run, so the call
		// still hits the API. We just want to confirm dry-run doesn't trip
		// the confirmation prompt under --no-input.
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": false,
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected dry-run to bypass confirmation, got %v", err)
	}
}

func TestRefund_CancelledByPromptDeclineNotReached(t *testing.T) {
	// With NoInput(true) and no --yes the prompt errors before we reach the API.
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API when confirmation is refused")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected confirmation error")
	}
}

func TestRefund_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--cancel-subscription"})
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

func TestRefund_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--cancel-subscription"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully refunded purchase number 123\t123\tcancelled\t"
	if strings.TrimSpace(out) != strings.TrimSpace(want) {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestRefund_APIErrorSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Refund amount cannot be greater than the purchase price.",
		})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "50.00"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Refund amount cannot be greater") {
		t.Fatalf("expected API error message, got: %v", err)
	}
}
