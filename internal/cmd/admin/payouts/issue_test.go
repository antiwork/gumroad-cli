package payouts

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestIssue_RequiresEmail(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--through", "2020-01-01", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIssue_RequiresThrough(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--email", "seller@example.com", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--through") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIssue_RequiresProcessor(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIssue_RejectsInvalidProcessor(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "wire"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "stripe") {
		t.Fatalf("expected processor validation error, got %v", err)
	}
}

func TestIssue_SplitRequiresPaypal(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe", "--split"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --split + stripe to fail client-side")
	}
	if !strings.Contains(err.Error(), "--split requires --processor paypal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIssue_RejectsFutureThrough(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2099-01-01", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "past") {
		t.Fatalf("expected past-date error, got %v", err)
	}
}

func TestIssue_RejectsBadDateFormat(t *testing.T) {
	cmd := newIssueCmd()
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "04/30/2026", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("expected date format error, got %v", err)
	}
}

func TestIssue_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newIssueCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestIssue_SendsRequestAndShowsResult(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body issueRequest

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
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"payout": map[string]any{
				"external_id":  "pay_abc",
				"amount_cents": 5000,
				"currency":     "usd",
				"state":        "processing",
				"processor":    "PAYPAL",
			},
		})
	})

	cmd := testutil.Command(newIssueCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "PayPal", "--split"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/payouts/issue" {
		t.Fatalf("got %s %s, want POST /internal/admin/payouts/issue", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.Email != "seller@example.com" || body.PayoutProcessor != "paypal" || body.PayoutPeriodEndDate != "2020-01-01" || !body.ShouldSplitTheAmount {
		t.Fatalf("unexpected body: %+v", body)
	}
	for _, want := range []string{"pay_abc", "5000 USD cents", "processing"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %q", want, out)
		}
	}
}

func TestIssue_ServerErrorSurfacesMessageAndNonZeroExit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "No payment was created",
		})
	})

	cmd := testutil.Command(newIssueCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected server error to surface")
	}
	if !strings.Contains(err.Error(), "No payment was created") {
		t.Errorf("missing underlying message: %v", err)
	}
}

func TestIssue_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"payout":  map[string]any{"external_id": "pay_abc", "amount_cents": 5000},
		})
	})

	cmd := testutil.Command(newIssueCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp issueResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Payout.ExternalID != "pay_abc" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestIssue_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"payout": map[string]any{
				"external_id":  "pay_abc",
				"amount_cents": 5000,
				"currency":     "usd",
				"state":        "processing",
				"processor":    "stripe",
			},
		})
	})

	cmd := testutil.Command(newIssueCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "pay_abc") || !strings.Contains(out, "5000 USD cents") || !strings.Contains(out, "processing") {
		t.Errorf("unexpected plain output: %q", out)
	}
}

func TestIssue_RendersAmountWithoutCurrency(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"payout":  map[string]any{"external_id": "pay_abc", "amount_cents": 5000, "state": "processing"},
		})
	})

	cmd := testutil.Command(newIssueCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "5000 cents") {
		t.Errorf("expected currency-less amount formatting: %q", out)
	}
}

func TestIssue_CancelledByUser(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not POST when user declines")
	})

	cmd := testutil.Command(newIssueCmd(), testutil.NoInput(true), testutil.DryRun(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "stripe"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation-required error without --yes")
	}
}

func TestIssue_DryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the issue endpoint")
	})

	cmd := testutil.Command(newIssueCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--through", "2020-01-01", "--processor", "paypal", "--split"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"POST", "/internal/admin/payouts/issue", "email: seller@example.com", "payout_processor: paypal", "payout_period_end_date: 2020-01-01", "should_split_the_amount: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q: %q", want, out)
		}
	}
}
