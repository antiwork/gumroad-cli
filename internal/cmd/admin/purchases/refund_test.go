package purchases

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func adminRefundHandler(t *testing.T, lookup, refund http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/refund"):
			t.Fatalf("unexpected GET to %s", r.URL.Path)
		case r.Method == http.MethodGet:
			if lookup == nil {
				t.Fatalf("unexpected lookup GET to %s", r.URL.Path)
			}
			lookup(w, r)
		case r.Method == http.MethodPost:
			if refund == nil {
				t.Fatalf("unexpected POST to %s", r.URL.Path)
			}
			refund(w, r)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}
}

func purchaseLookupResponder(currency string) http.HandlerFunc {
	return purchaseLookupResponderWithRefundable(currency, 0)
}

func purchaseLookupResponderWithRefundable(currency string, refundableCents int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"id":            "123",
			"email":         "buyer@example.com",
			"currency_type": currency,
		}
		if refundableCents > 0 {
			payload["amount_refundable_cents"] = refundableCents
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"purchase": payload})
	}
}

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

	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
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
	}))

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

func TestRefund_PartialLooksUpPurchaseAndConvertsUSD(t *testing.T) {
	var lookupHits int
	var body refundRequest
	var refundPath string

	testutil.SetupAdmin(t, adminRefundHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			lookupHits++
			if r.URL.Path != "/internal/admin/purchases/123" {
				t.Fatalf("unexpected lookup path %q", r.URL.Path)
			}
			purchaseLookupResponder("usd")(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			refundPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			testutil.JSON(t, w, map[string]any{
				"message":                "Successfully refunded purchase number 123",
				"purchase":               map[string]any{"id": "123"},
				"subscription_cancelled": false,
			})
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "5.00"})
	testutil.MustExecute(t, cmd)

	if lookupHits != 1 {
		t.Fatalf("expected one lookup, got %d", lookupHits)
	}
	if refundPath != "/internal/admin/purchases/123/refund" {
		t.Fatalf("unexpected refund path %q", refundPath)
	}
	if body.AmountCents != 500 {
		t.Errorf("got amount_cents=%d, want 500 for $5.00 USD", body.AmountCents)
	}
}

func TestRefund_PartialUsesPurchaseCurrencyForJPY(t *testing.T) {
	var body refundRequest

	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponder("jpy"),
		func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			testutil.JSON(t, w, map[string]any{
				"message":                "Successfully refunded purchase number 123",
				"purchase":               map[string]any{"id": "123"},
				"subscription_cancelled": false,
			})
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "500"})
	testutil.MustExecute(t, cmd)

	if body.AmountCents != 500 {
		t.Errorf("got amount_cents=%d, want 500 for ¥500 JPY (single-unit currency)", body.AmountCents)
	}
}

func TestRefund_RejectsMissingCurrencyTypeFromLookup(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"purchase": map[string]any{"id": "123", "email": "buyer@example.com"},
			})
		},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("refund POST must not fire when currency cannot be determined")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "500"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when currency_type is missing from lookup")
	}
	if !strings.Contains(err.Error(), "could not determine purchase currency") {
		t.Errorf("expected currency-missing guard, got: %v", err)
	}
	if strings.Contains(err.Error(), "Verify status") {
		t.Errorf("guard fires before the POST so it must not include the verify-state hint: %v", err)
	}
}

func TestRefund_RejectsEmptyCurrencyTypeFromLookup(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"purchase": map[string]any{
					"id":            "123",
					"email":         "buyer@example.com",
					"currency_type": "",
				},
			})
		},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("refund POST must not fire when currency is empty")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "500"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not determine purchase currency") {
		t.Fatalf("expected currency-empty guard, got: %v", err)
	}
}

func TestRefund_RejectsDecimalAmountForJPY(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponder("jpy"),
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach refund API for invalid amount")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "5.00"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "JPY") {
		t.Fatalf("expected JPY decimal-rejection error, got: %v", err)
	}
}

func TestRefund_RejectsInvalidAmount(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponder("usd"),
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach refund API")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "abc"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a valid amount") {
		t.Fatalf("expected validation error, got: %v", err)
	}
}

func TestRefund_RejectsZeroAmount(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponder("usd"),
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach refund API")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--amount must be greater than 0") {
		t.Fatalf("expected zero-amount error, got: %v", err)
	}
}

func TestRefund_ForwardsForceAndCancelSubscription(t *testing.T) {
	var body refundRequest

	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	}))

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
	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                   "Successfully refunded purchase number 123",
			"purchase":                  map[string]any{"id": "123"},
			"subscription_cancelled":    false,
			"subscription_cancel_error": "stripe blew up",
		})
	}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--cancel-subscription"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Subscription cancel failed: stripe blew up") {
		t.Errorf("expected cancel failure message: %q", out)
	}
}

func TestRefund_DryRunDoesNotContactRefundEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the refund endpoint")
	}))

	cmd := testutil.Command(newRefundCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/refund") {
		t.Errorf("expected dry-run preview to mention POST and the refund path, got: %q", out)
	}
	if !strings.Contains(out, "email: buyer@example.com") {
		t.Errorf("expected dry-run preview to include email, got: %q", out)
	}
}

func TestRefund_DryRunWithPartialAmountLooksUpButDoesNotRefund(t *testing.T) {
	var lookupHits int

	testutil.SetupAdmin(t, adminRefundHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			lookupHits++
			purchaseLookupResponder("jpy")(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("dry-run must not POST to the refund endpoint")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "500"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if lookupHits != 1 {
		t.Fatalf("expected one lookup to derive currency, got %d", lookupHits)
	}
	if !strings.Contains(out, "amount_cents: 500") {
		t.Errorf("expected dry-run preview to show amount_cents=500 (JPY-aware), got: %q", out)
	}
}

func TestRefund_CancelledByPromptDeclineNotReached(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API when confirmation is refused")
	}))

	cmd := testutil.Command(newRefundCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected confirmation error")
	}
}

func TestRefund_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	}))

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
	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                "Successfully refunded purchase number 123",
			"purchase":               map[string]any{"id": "123"},
			"subscription_cancelled": true,
		})
	}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--cancel-subscription"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully refunded purchase number 123\t123\tcancelled\t"
	if strings.TrimSpace(out) != strings.TrimSpace(want) {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestRefund_RejectsAmountAboveRefundableBalance(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponderWithRefundable("usd", 500),
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach refund API when amount exceeds refundable balance")
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "10.00"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "exceeds the refundable balance") {
		t.Fatalf("expected refundable-balance error, got: %v", err)
	}
}

func TestRefund_AcceptsAmountAtRefundableBalance(t *testing.T) {
	var body refundRequest
	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponderWithRefundable("usd", 500),
		func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			testutil.JSON(t, w, map[string]any{
				"message":                "Successfully refunded purchase number 123",
				"purchase":               map[string]any{"id": "123"},
				"subscription_cancelled": false,
			})
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "5.00"})
	testutil.MustExecute(t, cmd)

	if body.AmountCents != 500 {
		t.Errorf("got amount_cents=%d, want 500", body.AmountCents)
	}
}

func TestRefund_PurchaseHasNoChargeSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Purchase has no charge to refund",
		})
	}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
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

func TestRefund_APIErrorSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, adminRefundHandler(t,
		purchaseLookupResponder("usd"),
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": "Refund amount cannot be greater than the purchase price.",
			})
		}))

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--amount", "50.00"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected API error to surface")
	}
	if !strings.Contains(err.Error(), "Refund amount cannot be greater") {
		t.Errorf("missing underlying message: %v", err)
	}
	if !strings.Contains(err.Error(), "Verify status with 'gumroad admin purchases view 123'") {
		t.Errorf("expected verify-state hint: %v", err)
	}
}
