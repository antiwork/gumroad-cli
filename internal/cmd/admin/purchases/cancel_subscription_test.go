package purchases

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestCancelSubscription_RequiresEmail(t *testing.T) {
	cmd := newCancelSubscriptionCmd()
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancelSubscription_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestCancelSubscription_DefaultsToBuyerInitiatedAndOmitsBySeller(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body cancelSubscriptionRequest
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
			"message":             "Successfully cancelled subscription for purchase number 123",
			"cancelled_at":        "2026-04-30T12:00:00Z",
			"cancelled_by_admin":  true,
			"cancelled_by_seller": false,
		})
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/cancel_subscription" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/cancel_subscription", gotMethod, gotPath)
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
	if _, present := bodyKeys["by_seller"]; present {
		t.Errorf("by_seller must be omitted when not set, got body keys: %v", bodyKeys)
	}
	if !strings.Contains(out, "Successfully cancelled subscription for purchase number 123") {
		t.Errorf("expected success message in output: %q", out)
	}
	if !strings.Contains(out, "Cancelled at: 2026-04-30T12:00:00Z") {
		t.Errorf("expected cancelled_at line in output: %q", out)
	}
}

func TestCancelSubscription_BySellerForwardsBoolean(t *testing.T) {
	var body cancelSubscriptionRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":             "Successfully cancelled subscription for purchase number 123",
			"cancelled_at":        "2026-04-30T12:00:00Z",
			"cancelled_by_admin":  true,
			"cancelled_by_seller": true,
		})
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--by-seller"})
	testutil.MustExecute(t, cmd)

	if !body.BySeller {
		t.Errorf("expected by_seller=true in body, got %#v", body)
	}
}

func TestCancelSubscription_AlreadyCancelledShortCircuit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":             "already_cancelled",
			"message":            "Subscription is already cancelled",
			"cancelled_at":       "2026-04-29T09:00:00Z",
			"cancelled_by_admin": false,
		})
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Subscription is already cancelled", "Status: already_cancelled", "Cancelled at: 2026-04-29T09:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestCancelSubscription_AlreadyInactiveSurfacesTerminationReasonAndDeactivatedAt(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":             "already_inactive",
			"message":            "Subscription is no longer active",
			"termination_reason": "failed_payment",
			"deactivated_at":     "2026-04-28T11:00:00Z",
		})
	}

	testutil.SetupAdmin(t, handler)
	humanCmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true), testutil.Quiet(false))
	humanCmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	humanOut := testutil.CaptureStdout(func() { testutil.MustExecute(t, humanCmd) })

	for _, want := range []string{
		"Subscription is no longer active",
		"Status: already_inactive",
		"Termination reason: failed_payment",
		"Deactivated at: 2026-04-28T11:00:00Z",
	} {
		if !strings.Contains(humanOut, want) {
			t.Errorf("output missing %q: %q", want, humanOut)
		}
	}
	if strings.Contains(humanOut, "Cancelled at:") {
		t.Errorf("must not print 'Cancelled at:' when only deactivated_at is set: %q", humanOut)
	}

	testutil.SetupAdmin(t, handler)
	plainCmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true), testutil.PlainOutput())
	plainCmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	plainOut := testutil.CaptureStdout(func() { testutil.MustExecute(t, plainCmd) })

	want := "true\tSubscription is no longer active\t123\talready_inactive\t2026-04-28T11:00:00Z"
	if strings.TrimSpace(plainOut) != want {
		t.Fatalf("unexpected plain output: %q", plainOut)
	}
}

func TestCancelSubscription_DryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the cancel_subscription endpoint")
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--by-seller"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/cancel_subscription") {
		t.Errorf("expected dry-run preview to mention POST and the cancel_subscription path, got: %q", out)
	}
	if !strings.Contains(out, "email: buyer@example.com") || !strings.Contains(out, "by_seller: true") {
		t.Errorf("expected dry-run preview to include email and by_seller, got: %q", out)
	}
}

func TestCancelSubscription_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":              "already_cancelled",
			"message":             "Subscription is already cancelled",
			"cancelled_at":        "2026-04-29T09:00:00Z",
			"cancelled_by_admin":  true,
			"cancelled_by_seller": false,
		})
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success           bool   `json:"success"`
		Status            string `json:"status"`
		Message           string `json:"message"`
		CancelledAt       string `json:"cancelled_at"`
		CancelledByAdmin  bool   `json:"cancelled_by_admin"`
		CancelledBySeller bool   `json:"cancelled_by_seller"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "already_cancelled" || resp.Message != "Subscription is already cancelled" || resp.CancelledAt != "2026-04-29T09:00:00Z" || !resp.CancelledByAdmin {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestCancelSubscription_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":             "Successfully cancelled subscription for purchase number 123",
			"cancelled_at":        "2026-04-30T12:00:00Z",
			"cancelled_by_admin":  true,
			"cancelled_by_seller": false,
		})
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully cancelled subscription for purchase number 123\t123\t\t2026-04-30T12:00:00Z"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestCancelSubscription_NoSubscriptionSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Purchase has no subscription",
		})
	})

	cmd := testutil.Command(newCancelSubscriptionCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error to surface")
	}
	if !strings.Contains(err.Error(), "Purchase has no subscription") {
		t.Errorf("missing underlying message: %v", err)
	}
}
