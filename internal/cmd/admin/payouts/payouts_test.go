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

	if gotMethod != "GET" || gotPath != "/internal/admin/payouts" {
		t.Fatalf("got %s %s, want GET /internal/admin/payouts", gotMethod, gotPath)
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
