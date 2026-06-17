package payouts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestListUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotEmail, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"recent_payouts": []map[string]any{
				{
					"external_id": "pay_123", "amount_cents": 5000, "currency": "usd",
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "stripe", "bank_account_visual": "****1234",
				},
			},
			"pagination":              map[string]any{"next": nil, "limit": 20},
			"next_payout_date":        "2026-04-30",
			"balance_for_next_payout": "$25.00",
			"payout_note":             "Manual review",
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/payouts" {
		t.Fatalf("got %s %s, want GET /internal/admin/payouts", gotMethod, gotPath)
	}
	if gotEmail != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", gotEmail)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{"seller@example.com", "User ID: 2245593582708", "Next payout: 2026-04-30", "$25.00", "Manual review", "pay_123", "5000 USD cents"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestListPassesLimitAndCursor(t *testing.T) {
	var gotQuery string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{"recent_payouts": []any{}})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--user-id", "abc", "--limit", "5", "--cursor", "cur-1"})
	testutil.MustExecute(t, cmd)

	for _, want := range []string{"user_id=abc", "limit=5", "cursor=cur-1"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %q: %q", want, gotQuery)
		}
	}
}

func TestListShowsNextCursorFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id": "abc",
			"recent_payouts": []map[string]any{
				{"external_id": "pay_1", "amount_cents": 100, "state": "completed", "processor": "stripe"},
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 2},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--user-id", "abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "More results: --cursor cur-next") {
		t.Fatalf("expected next-cursor footer, got: %q", out)
	}
}

func TestListRejectsZeroLimit(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{"--user-id", "abc", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected limit validation error, got %v", err)
	}
}

func TestListDoesNotRepeatUserIDWhenUsedAsLookup(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Contains(out, "User ID: 2245593582708") {
		t.Fatalf("must not repeat user_id immediately after the headline: %q", out)
	}
	if strings.Count(out, "2245593582708") != 1 {
		t.Fatalf("expected user_id to appear once, got: %q", out)
	}
}

func TestListRequiresEmailOrUserID(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "supply --email or --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"recent_payouts": []map[string]any{{"external_id": "pay_123"}},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		RecentPayouts []map[string]any `json:"recent_payouts"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.RecentPayouts) != 1 || resp.RecentPayouts[0]["external_id"] != "pay_123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestListPlainOutputWithPaypalDestination(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"recent_payouts": []map[string]any{
				{
					"external_id": "pay_123", "amount_cents": 5000,
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "paypal", "paypal_email": "seller@example.com",
				},
			},
			"next_payout_date":        "2026-04-30",
			"balance_for_next_payout": "$25.00",
			"payout_note":             "Manual review",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tpay_123\t5000 cents\tcompleted\t2026-04-24T12:00:00Z\tpaypal\tseller@example.com\t2026-04-30\t$25.00\tManual review"
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

	want := "seller@example.com\t\t\t\t\t\t\t2026-04-30\t$25.00\tManual review"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestListRendersStripeTransferAndBankAccountDetails(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"recent_payouts": []map[string]any{
				{
					"external_id": "pay_123", "amount_cents": 5000, "currency": "usd",
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "stripe", "bank_account_visual": "******6789",
					"trace_id": nil, "stripe_transfer_id": "po_1Test",
					"bank_account": map[string]any{
						"bank_number":              "110000000",
						"account_holder_full_name": "Stripe Test Account",
						"account_type":             "checking",
						"currency":                 "usd",
					},
				},
			},
			"pagination": map[string]any{"next": nil, "limit": 20},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"pay_123",
		"Details:",
		"stripe transfer: po_1Test",
		"routing/BIC: 110000000",
		"account holder: Stripe Test Account",
		"account type: checking",
		"currency: USD",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "trace:") {
		t.Fatalf("null trace_id must not render: %q", out)
	}
}

func TestListOmitsDetailsForPayoutsWithoutBankOrTransfer(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"recent_payouts": []map[string]any{
				{
					"external_id": "pay_pp", "amount_cents": 5000,
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "paypal", "paypal_email": "payme@example.com",
					"trace_id": nil, "stripe_transfer_id": nil, "bank_account": nil,
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "pay_pp") {
		t.Fatalf("expected payout row in output: %q", out)
	}
	if strings.Contains(out, "Details:") {
		t.Fatalf("payouts without bank account or transfer id must not render a Details section: %q", out)
	}
}

func TestListPlainOutputCarriesStripeTransferAndBankFields(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"recent_payouts": []map[string]any{
				{
					"external_id": "pay_123", "amount_cents": 5000, "currency": "usd",
					"state": "completed", "created_at": "2026-04-24T12:00:00Z",
					"processor": "stripe", "bank_account_visual": "******6789",
					"trace_id": nil, "stripe_transfer_id": "po_1Test",
					"bank_account": map[string]any{
						"bank_number":              "110000000",
						"account_holder_full_name": "Stripe Test Account",
						"account_type":             "checking",
						"currency":                 "usd",
					},
				},
			},
			"next_payout_date":        "2026-04-30",
			"balance_for_next_payout": "$25.00",
			"payout_note":             "Manual review",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tpay_123\t5000 USD cents\tcompleted\t2026-04-24T12:00:00Z\tstripe\t******6789\t2026-04-30\t$25.00\tManual review\tpo_1Test\t110000000\tStripe Test Account\tchecking\tUSD"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestFormatAmountTrimsCurrency(t *testing.T) {
	p := payout{AmountCents: 5000, Currency: " usd "}
	if got := formatAmount(p); got != "5000 USD cents" {
		t.Fatalf("got %q, want 5000 USD cents", got)
	}
}

func TestNewPayoutsCmdWiresAllSubcommands(t *testing.T) {
	cmd := NewPayoutsCmd()
	if cmd.Use != "payouts" {
		t.Fatalf("Use = %q, want payouts", cmd.Use)
	}
	want := map[string]bool{"list": false, "pause": false, "resume": false, "issue": false, "scheduled": false}
	for _, sub := range cmd.Commands() {
		if _, ok := want[sub.Use]; !ok {
			t.Fatalf("unexpected subcommand %q", sub.Use)
		}
		want[sub.Use] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}
