package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func sampleInfoPayload() map[string]any {
	return map[string]any{
		"user": map[string]any{
			"id":          "u_abc",
			"email":       "seller@example.com",
			"name":        "Seller One",
			"username":    "sellerone",
			"profile_url": "https://sellerone.gumroad.com/",
			"country":     "Germany",
			"created_at":  "2024-01-01T00:00:00Z",
			"deleted_at":  nil,
			"risk_state": map[string]any{
				"status":                    "Compliant",
				"user_risk_state":           "compliant",
				"suspended":                 false,
				"flagged_for_fraud":         false,
				"flagged_for_tos_violation": false,
				"on_probation":              false,
				"compliant":                 true,
				"last_status_changed_at":    nil,
			},
			"two_factor_authentication_enabled": true,
			"payouts": map[string]any{
				"paused_internally":       false,
				"paused_by_user":          false,
				"paused_by_source":        nil,
				"paused_for_reason":       nil,
				"next_payout_date":        "2026-05-15",
				"balance_for_next_payout": "$120.50",
			},
			"stats": map[string]any{
				"sales_count":              42,
				"total_earnings_formatted": "$1,234.56",
				"unpaid_balance_formatted": "$120.50",
				"comments_count":           3,
			},
		},
	}
}

func TestInfoRequiresEmail(t *testing.T) {
	cmd := newInfoCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInfoUsesInternalAdminEndpointAndRendersHumanOutput(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var body infoRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, sampleInfoPayload())
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/info" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/info", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("email must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.Email != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", body.Email)
	}
	for _, want := range []string{
		"Seller One",
		"Email: seller@example.com",
		"Username: sellerone",
		"Profile: https://sellerone.gumroad.com/",
		"Country: Germany",
		"Created: 2024-01-01T00:00:00Z",
		"Risk: Compliant",
		"Two-factor: enabled",
		"Payouts: active",
		"next payout: 2026-05-15",
		"balance for next payout: $120.50",
		"Sales: 42",
		"Total earnings: $1,234.56",
		"Unpaid balance: $120.50",
		"Comments: 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestInfoFlagsSuspendedUserAndPausedPayouts(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	risk := user["risk_state"].(map[string]any)
	risk["status"] = "Suspended"
	risk["user_risk_state"] = "suspended_for_fraud"
	risk["suspended"] = true
	risk["flagged_for_fraud"] = true
	risk["compliant"] = false
	risk["last_status_changed_at"] = "2026-04-15T12:00:00Z"
	payouts := user["payouts"].(map[string]any)
	payouts["paused_internally"] = true
	payouts["paused_by_source"] = "admin"
	payouts["paused_for_reason"] = "Manual review pending"

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Risk: Suspended",
		"user_risk_state: suspended_for_fraud",
		"suspended: true",
		"flagged_for_fraud: true",
		"last status change: 2026-04-15T12:00:00Z",
		"Payouts: paused (internal)",
		"paused by: admin",
		"reason: Manual review pending",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestInfoMarksSelfPausedPayouts(t *testing.T) {
	payload := sampleInfoPayload()
	payouts := payload["user"].(map[string]any)["payouts"].(map[string]any)
	payouts["paused_by_user"] = true
	payouts["paused_by_source"] = "user"

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Payouts: paused (by user)") {
		t.Errorf("expected 'Payouts: paused (by user)' line, got: %q", out)
	}
	if !strings.Contains(out, "paused by: user") {
		t.Errorf("expected 'paused by: user' line, got: %q", out)
	}
}

func TestInfoPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleInfoPayload())
	})

	cmd := testutil.Command(newInfoCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tSeller One\tsellerone\tCompliant\ttrue\tfalse\tfalse\t2026-05-15\t42\t$1,234.56\t2024-01-01T00:00:00Z"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestInfoJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleInfoPayload())
	})

	cmd := testutil.Command(newInfoCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool                   `json:"success"`
		User    map[string]interface{} `json:"user"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success {
		t.Fatalf("expected success=true: %s", out)
	}
	if resp.User["id"] != "u_abc" {
		t.Errorf("expected user.id=u_abc, got %v", resp.User["id"])
	}
	if resp.User["country"] != "Germany" {
		t.Errorf("expected user.country=Germany, got %v", resp.User["country"])
	}
	stats, ok := resp.User["stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected stats object, got %T", resp.User["stats"])
	}
	if stats["total_earnings_formatted"] != "$1,234.56" {
		t.Errorf("expected stats.total_earnings_formatted=$1,234.56, got %v", stats["total_earnings_formatted"])
	}
}

func TestInfoUserNotFoundSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "User not found",
		})
	})

	cmd := testutil.Command(newInfoCmd())
	cmd.SetArgs([]string{"--email", "missing@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected user-not-found error")
	}
	if !strings.Contains(err.Error(), "User not found") {
		t.Errorf("missing underlying message: %v", err)
	}
}
