package payouts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestListUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotEmail, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		var payload listRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotEmail = payload.Email
		testutil.JSON(t, w, map[string]any{
			"last_payouts": []map[string]any{
				{
					"external_id": "pay_123", "amount_cents": 5000, "currency": "usd",
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "stripe", "bank_account_visual": "****1234",
				},
			},
			"next_payout_date":        "2026-04-30",
			"balance_for_next_payout": "$25.00",
			"payout_note":             "Manual review",
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/payouts/list" {
		t.Fatalf("got %s %s, want POST /internal/admin/payouts/list", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("email should not be sent in query string, got %q", gotQuery)
	}
	if gotEmail != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", gotEmail)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{"seller@example.com", "Next payout: 2026-04-30", "$25.00", "Manual review", "pay_123", "5000 USD cents"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestListRequiresEmail(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"last_payouts": []map[string]any{{"external_id": "pay_123"}},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		LastPayouts []map[string]any `json:"last_payouts"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.LastPayouts) != 1 || resp.LastPayouts[0]["external_id"] != "pay_123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestListPlainOutputWithPaypalDestination(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"last_payouts": []map[string]any{
				{
					"external_id": "pay_123", "amount_cents": 5000,
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "paypal", "paypal_email": "seller@example.com",
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tpay_123\t5000 cents\tcompleted\t2026-04-24T12:00:00Z\tpaypal\tseller@example.com"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestListPlainOutputWithNoPayouts(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"next_payout_date":        "2026-04-30",
			"balance_for_next_payout": "$25.00",
			"payout_note":             "Manual review",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\t\t\t\t\t2026-04-30\t$25.00\tManual review"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestNewPayoutsCmdWiresList(t *testing.T) {
	cmd := NewPayoutsCmd()
	if cmd.Use != "payouts" {
		t.Fatalf("Use = %q, want payouts", cmd.Use)
	}
	if got := cmd.Commands(); len(got) != 1 || got[0].Use != "list" {
		t.Fatalf("unexpected subcommands: %#v", got)
	}
}
