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
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundAllForFraudRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestRefundAllForFraudSendsRequestAndRendersSummary(t *testing.T) {
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
		if strings.Contains(string(raw), `"force"`) {
			t.Errorf("force must be omitted when the flag is not passed, got %q", raw)
		}
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"user_id":        "2245593582708",
			"refunded_count": 17,
			"skipped_count":  1,
			"failed":         []any{},
		})
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/refund_all_for_fraud" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/refund_all_for_fraud", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.Force {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User ID: 2245593582708", "Refunded 17, skipped 1 already-refunded, 0 failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRefundAllForFraudForwardsForceAndExpectedEmail(t *testing.T) {
	var body refundAllForFraudRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"user_id":        "2245593582708",
			"refunded_count": 0,
			"skipped_count":  0,
			"failed":         []any{},
		})
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--force"})
	testutil.MustExecute(t, cmd)

	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || !body.Force {
		t.Fatalf("unexpected request body: %#v", body)
	}
}

func TestRefundAllForFraudPartialFailureRendersTableAndExitsNonZero(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"user_id": "2245593582708",
			"refunded_count": 16,
			"skipped_count": 1,
			"failed": [
				{"purchase_external_id_numeric": 12345, "error": "Stripe is unavailable"},
				{"purchase_external_id_numeric": 67890, "error": "Refund amount cannot be greater than the purchase price."}
			]
		}`)
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	var execErr error
	out := testutil.CaptureStdout(func() { execErr = cmd.Execute() })

	if execErr == nil {
		t.Fatal("expected non-nil error when some purchases failed")
	}
	if !strings.Contains(execErr.Error(), "2 purchase(s) failed to refund") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	for _, want := range []string{"Refunded 16, skipped 1 already-refunded, 2 failed", "12345", "Stripe is unavailable", "67890"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRefundAllForFraudJSONPreservesResponseAndExitsNonZeroOnFailures(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"user_id": "2245593582708",
			"refunded_count": 0,
			"skipped_count": 0,
			"failed": [{"purchase_external_id_numeric": 12345, "error": "Stripe is unavailable"}]
		}`)
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	var execErr error
	out := testutil.CaptureStdout(func() { execErr = cmd.Execute() })

	if execErr == nil {
		t.Fatal("expected non-nil error when some purchases failed in JSON mode")
	}

	var resp refundAllForFraudResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || len(resp.Failed) != 1 || resp.Failed[0].PurchaseExternalIDNumeric != 12345 {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundAllForFraudJSONSuccessExitsZero(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"user_id":        "2245593582708",
			"refunded_count": 3,
			"skipped_count":  0,
			"failed":         []any{},
		})
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp refundAllForFraudResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.RefundedCount != 3 {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundAllForFraudDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the refund_all_for_fraud endpoint")
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--force"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/refund_all_for_fraud") {
		t.Errorf("expected dry-run preview to mention POST and the refund_all_for_fraud path, got: %q", out)
	}
	if !strings.Contains(out, "user_id: 2245593582708") || !strings.Contains(out, "force: true") {
		t.Errorf("expected dry-run preview to include user_id and force, got: %q", out)
	}
}

func TestRefundAllForFraudPlainOutputIncludesFailureRows(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"user_id": "2245593582708",
			"refunded_count": 1,
			"skipped_count": 0,
			"failed": [{"purchase_external_id_numeric": 12345, "error": "Stripe is unavailable"}]
		}`)
	})

	cmd := testutil.Command(newRefundAllForFraudCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	var execErr error
	out := testutil.CaptureStdout(func() { execErr = cmd.Execute() })

	if execErr == nil {
		t.Fatal("expected non-nil error when some purchases failed in plain mode")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected summary row plus one failure row, got %d lines: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "false\t") || !strings.Contains(lines[0], "2245593582708") {
		t.Fatalf("unexpected summary row: %q", lines[0])
	}
	if lines[1] != "failed\t12345\tStripe is unavailable" {
		t.Fatalf("unexpected failure row: %q", lines[1])
	}
}
