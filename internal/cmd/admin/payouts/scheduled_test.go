package payouts

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestScheduledList_Default(t *testing.T) {
	var gotMethod, gotPath string
	var body scheduledListRequest
	var bodyKeys map[string]json.RawMessage

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
			t.Fatalf("decode keys: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"scheduled_payouts": []map[string]any{
				{"external_id": "pay_1", "email": "seller@example.com", "amount_cents": 1000, "currency": "usd", "status": "flagged", "processor": "stripe", "scheduled_at": "2026-05-01"},
			},
			"limit": 20,
		})
	})

	cmd := testutil.Command(newScheduledListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/payouts/scheduled_list" {
		t.Fatalf("got %s %s, want POST /internal/admin/payouts/scheduled_list", gotMethod, gotPath)
	}
	if _, hasStatus := bodyKeys["status"]; hasStatus {
		t.Errorf("status must be omitted when not provided, got: %v", bodyKeys)
	}
	if _, hasLimit := bodyKeys["limit"]; hasLimit {
		t.Errorf("limit must be omitted when not provided, got: %v", bodyKeys)
	}
	for _, want := range []string{"pay_1", "flagged", "1000 USD cents"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestScheduledList_PassesStatusAndLimit(t *testing.T) {
	var body scheduledListRequest
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		testutil.JSON(t, w, map[string]any{"scheduled_payouts": []any{}, "limit": 50})
	})

	cmd := testutil.Command(newScheduledListCmd())
	cmd.SetArgs([]string{"--status", "FLAGGED", "--limit", "50"})
	testutil.MustExecute(t, cmd)

	if body.Status != "flagged" || body.Limit != 50 {
		t.Fatalf("got body=%+v, want status=flagged limit=50", body)
	}
}

func TestScheduledList_RejectsBadStatus(t *testing.T) {
	cmd := newScheduledListCmd()
	cmd.SetArgs([]string{"--status", "bogus"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--status must be one of") {
		t.Fatalf("expected status validation error, got %v", err)
	}
}

func TestScheduledList_RejectsZeroLimit(t *testing.T) {
	cmd := newScheduledListCmd()
	cmd.SetArgs([]string{"--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected limit validation error, got %v", err)
	}
}

func TestScheduledList_EmptyStateMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"scheduled_payouts": []any{}, "limit": 20})
	})

	cmd := testutil.Command(newScheduledListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--status", "flagged"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No scheduled payouts found") {
		t.Errorf("expected empty-state message, got: %q", out)
	}
}

func TestScheduledExecute_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newScheduledExecuteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"pay_abc"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
}

func TestScheduledExecute_HappyPath(t *testing.T) {
	var gotMethod, gotPath string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"result":           "executed",
			"message":          "Scheduled payout pay_abc executed",
			"scheduled_payout": map[string]any{"external_id": "pay_abc", "status": "executed"},
		})
	})

	cmd := testutil.Command(newScheduledExecuteCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"pay_abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/payouts/pay_abc/scheduled_execute" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{"Scheduled payout pay_abc executed", "Result: executed", "Status: executed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestScheduledExecute_ServerErrorSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Scheduled payout already executed",
		})
	})

	cmd := testutil.Command(newScheduledExecuteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"pay_abc"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from 422")
	}
	if !strings.Contains(err.Error(), "Scheduled payout already executed") {
		t.Errorf("missing underlying message: %v", err)
	}
}

func TestScheduledCancel_HappyPath(t *testing.T) {
	var gotPath string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"message":          "Cancelled",
			"scheduled_payout": map[string]any{"external_id": "pay_abc", "status": "cancelled"},
		})
	})

	cmd := testutil.Command(newScheduledCancelCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"pay_abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/internal/admin/payouts/pay_abc/scheduled_cancel" {
		t.Fatalf("got path %q", gotPath)
	}
	for _, want := range []string{"Cancelled", "Status: cancelled"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestScheduledCancel_ServerError(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Cannot cancel an executed payout",
		})
	})

	cmd := testutil.Command(newScheduledCancelCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"pay_abc"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Cannot cancel an executed payout") {
		t.Fatalf("expected server message in error, got %v", err)
	}
}

func TestScheduledList_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"scheduled_payouts": []map[string]any{
				{"external_id": "pay_1", "email": "seller@example.com", "amount_cents": 1000, "status": "flagged", "processor": "stripe", "scheduled_at": "2026-05-01", "created_at": "2026-04-30"},
			},
		})
	})

	cmd := testutil.Command(newScheduledListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "pay_1\tseller@example.com\t1000 cents\tflagged\tstripe\t2026-05-01\t2026-04-30"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestScheduledList_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"scheduled_payouts": []map[string]any{{"external_id": "pay_1"}},
			"limit":             20,
		})
	})

	cmd := testutil.Command(newScheduledListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp scheduledListResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.ScheduledPayouts) != 1 || resp.ScheduledPayouts[0].ExternalID != "pay_1" {
		t.Fatalf("unexpected JSON: %s", out)
	}
}

func TestScheduledList_EmptyDefaultMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"scheduled_payouts": []any{}})
	})

	cmd := testutil.Command(newScheduledListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No scheduled payouts found.") {
		t.Errorf("expected default empty-state, got %q", out)
	}
}

func TestScheduledExecute_DryRun(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newScheduledExecuteCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"pay_abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/payouts/pay_abc/scheduled_execute") {
		t.Errorf("dry-run output unexpected: %q", out)
	}
}

func TestScheduledExecute_PlainAndJSON(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"result":           "executed",
			"message":          "Done",
			"scheduled_payout": map[string]any{"external_id": "pay_abc", "status": "executed"},
		})
	}

	testutil.SetupAdmin(t, handler)
	plain := testutil.Command(newScheduledExecuteCmd(), testutil.Yes(true), testutil.PlainOutput())
	plain.SetArgs([]string{"pay_abc"})
	plainOut := testutil.CaptureStdout(func() { testutil.MustExecute(t, plain) })
	if !strings.Contains(plainOut, "true\tDone\tpay_abc\texecuted\texecuted") {
		t.Errorf("unexpected plain: %q", plainOut)
	}

	testutil.SetupAdmin(t, handler)
	js := testutil.Command(newScheduledExecuteCmd(), testutil.Yes(true), testutil.JSONOutput())
	js.SetArgs([]string{"pay_abc"})
	jsOut := testutil.CaptureStdout(func() { testutil.MustExecute(t, js) })
	var resp scheduledExecuteResponse
	if err := json.Unmarshal([]byte(jsOut), &resp); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, jsOut)
	}
	if !resp.Success || resp.Result != "executed" {
		t.Fatalf("unexpected JSON: %s", jsOut)
	}
}

func TestScheduledCancel_DryRun(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newScheduledCancelCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"pay_abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/payouts/pay_abc/scheduled_cancel") {
		t.Errorf("dry-run output unexpected: %q", out)
	}
}

func TestScheduledCancel_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newScheduledCancelCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"pay_abc"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
}

func TestScheduledCancel_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":          true,
			"message":          "Cancelled",
			"scheduled_payout": map[string]any{"external_id": "pay_abc", "status": "cancelled"},
		})
	})

	cmd := testutil.Command(newScheduledCancelCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"pay_abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "true\tCancelled\tpay_abc\tcancelled") {
		t.Errorf("unexpected plain: %q", out)
	}
}

func TestScheduledCmdWiresChildren(t *testing.T) {
	cmd := newScheduledCmd()
	want := map[string]bool{"list": false, "execute <external_id>": false, "cancel <external_id>": false}
	for _, sub := range cmd.Commands() {
		if _, ok := want[sub.Use]; ok {
			want[sub.Use] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}
