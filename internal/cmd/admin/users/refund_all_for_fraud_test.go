package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRefundAllForFraudRequiresUserID(t *testing.T) {
	cmd := newRefundAllForFraudCmd()
	cmd.SetArgs([]string{"--expected-email", "seller@example.com", "--expected-count", "3"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundAllForFraudRequiresExpectedEmail(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without --expected-email")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-count", "3"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing --expected-email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --expected-email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundAllForFraudRequiresExpectedCount(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without --expected-count")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing --expected-count error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --expected-count") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundAllForFraudRejectsNegativeExpectedCount(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with a negative --expected-count")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected negative --expected-count error")
	}
	if !strings.Contains(err.Error(), "--expected-count must be a non-negative integer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundAllForFraudRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "3"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestRefundAllForFraudSendsRequestAndRendersQueuedSummary(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body refundAllForFraudRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if strings.Contains(string(raw), `"block_buyers"`) {
			t.Errorf("block_buyers must be omitted when the flag is not passed, got %q", raw)
		}
		testutil.JSON(t, w, map[string]any{
			"success":             true,
			"user_id":             "2245593582708",
			"status":              "queued",
			"message":             "Refund all for fraud queued",
			"purchases_to_refund": 18,
			"block_buyers":        false,
		})
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "18"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/refund_all_for_fraud" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/refund_all_for_fraud", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || body.ExpectedPurchaseCount != 18 || body.BlockBuyers {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{
		"Queued fraud refunds for 18 purchase(s); buyers will not be blocked",
		"User ID: 2245593582708",
		"Status: queued",
		"check the seller's comments and the admin audit log",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRefundAllForFraudForwardsBlockBuyers(t *testing.T) {
	var body refundAllForFraudRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"success":             true,
			"user_id":             "2245593582708",
			"status":              "queued",
			"message":             "Refund all for fraud queued",
			"purchases_to_refund": 4,
			"block_buyers":        true,
		})
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "4", "--block-buyers"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.UserID != "2245593582708" || !body.BlockBuyers || body.ExpectedPurchaseCount != 4 {
		t.Fatalf("unexpected request body: %#v", body)
	}
	if !strings.Contains(out, "buyers will be blocked") {
		t.Fatalf("output should note buyer blocking: %q", out)
	}
}

func TestRefundAllForFraudConflictExplainsStaleStateAndDuplicateRun(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		testutil.RawJSON(t, w, `{
			"success": false,
			"user_id": "2245593582708",
			"message": "Expected purchase count does not match the current number of refundable purchases",
			"current": {"purchases_to_refund": 17}
		}`)
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "18"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error on 409 conflict")
	}
	for _, want := range []string{
		"Expected purchase count does not match",
		"gumroad admin users info",
		"already be queued",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("conflict error missing %q: %v", want, err)
		}
	}
}

func TestRefundAllForFraudJSONPreservesQueuedResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"user_id": "2245593582708",
			"status": "queued",
			"message": "Refund all for fraud queued",
			"purchases_to_refund": 3,
			"block_buyers": false
		}`)
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "3"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp refundAllForFraudResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "queued" || resp.PurchasesToRefund != 3 || resp.BlockBuyers {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundAllForFraudDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the refund_all_for_fraud endpoint")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "18", "--block-buyers"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/refund_all_for_fraud") {
		t.Errorf("expected dry-run preview to mention POST and the refund_all_for_fraud path, got: %q", out)
	}
	for _, want := range []string{"user_id: 2245593582708", "expected_email: seller@example.com", "expected_purchase_count: 18", "block_buyers: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected dry-run preview to include %q, got: %q", want, out)
		}
	}
}

func TestRefundAllForFraudPlainOutputPrintsQueuedRow(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"user_id": "2245593582708",
			"status": "queued",
			"message": "Refund all for fraud queued",
			"purchases_to_refund": 2,
			"block_buyers": true
		}`)
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--expected-count", "2", "--block-buyers"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	line := strings.TrimSpace(out)
	if line != "true\tRefund all for fraud queued\t2245593582708\tqueued\t2\ttrue" {
		t.Fatalf("unexpected plain row: %q", line)
	}
}
