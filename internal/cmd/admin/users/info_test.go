package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func sampleInfoPayload() map[string]any {
	return map[string]any{
		"user_id": "2245593582708",
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

func TestInfoRequiresEmailOrUserID(t *testing.T) {
	cmd := newInfoCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "supply --email or --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInfoResolvesByUserID(t *testing.T) {
	var gotEmail, gotUserID string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, sampleInfoPayload())
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotUserID != "2245593582708" {
		t.Fatalf("got user_id %q, want 2245593582708", gotUserID)
	}
	if gotEmail != "" {
		t.Errorf("expected email to be empty, got %q", gotEmail)
	}
	if !strings.Contains(out, "Seller One") {
		t.Errorf("expected resolved user info in output: %q", out)
	}
}

func TestInfoSendsBothWhenEmailAndUserIDSupplied(t *testing.T) {
	var gotEmail, gotUserID string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, sampleInfoPayload())
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--user-id", "2245593582708"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotEmail != "seller@example.com" {
		t.Errorf("got email %q, want seller@example.com (server prefers user_id but CLI forwards both)", gotEmail)
	}
	if gotUserID != "2245593582708" {
		t.Errorf("got user_id %q, want 2245593582708", gotUserID)
	}
}

func TestInfoUsesInternalAdminEndpointAndRendersHumanOutput(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotEmail string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotEmail = r.URL.Query().Get("email")
		testutil.JSON(t, w, sampleInfoPayload())
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/info" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/info", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotEmail != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", gotEmail)
	}
	for _, want := range []string{
		"Seller One",
		"Email: seller@example.com",
		"User ID: 2245593582708",
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

func TestInfoRendersSignInAndSocialContext(t *testing.T) {
	output.SetColorEnabledForTesting(true)
	t.Cleanup(output.ResetColorEnabledForTesting)

	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["locale"] = "fr"
	user["timezone"] = "Eastern Time (US & Canada)"
	user["sign_in"] = map[string]any{
		"account_created_ip": "1.2.3.4",
		"current_ip":         "5.6.7.8",
		"current_at":         "2026-05-10T09:30:00Z",
		"last_ip":            "9.10.11.12",
		"last_at":            "2026-05-08T18:45:00Z",
		"count":              42,
	}
	user["social"] = map[string]any{
		"twitter_user_id": "1",
		"twitter_handle":  "alice",
		"facebook_uid":    "fb1",
		"google_uid":      "gid1",
		"oauth_provider":  "google_oauth2",
		"external_authentications": []map[string]any{
			{
				"provider":  "apple",
				"uid":       "001-test",
				"linked_at": "2026-05-09T12:00:00Z",
			},
		},
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Locale: fr",
		"Timezone: Eastern Time (US & Canada)",
		"Sign-in:",
		"account-created IP: 1.2.3.4",
		"current: 5.6.7.8 at 2026-05-10T09:30:00Z",
		"last: 9.10.11.12 at 2026-05-08T18:45:00Z",
		"count: 42",
		"Social:",
		"twitter: @alice (id: 1)",
		"facebook UID: fb1",
		"google UID: gid1",
		"OAuth provider: google_oauth2",
		"external authentications:",
		"\x1b[1mPROVIDER\x1b[0m",
		"apple",
		"001-test",
		"2026-05-09T12:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestFormatTwitterLabelsUserIDOnly(t *testing.T) {
	tests := []struct {
		name   string
		handle string
		userID string
		want   string
	}{
		{name: "handle and ID", handle: "alice", userID: "1", want: "@alice (id: 1)"},
		{name: "handle with at-prefix", handle: "@alice", userID: "1", want: "@alice (id: 1)"},
		{name: "handle only", handle: "alice", want: "@alice"},
		{name: "ID only", userID: "1", want: "(id: 1)"},
		{name: "empty", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTwitter(tt.handle, tt.userID); got != tt.want {
				t.Fatalf("formatTwitter(%q, %q) = %q, want %q", tt.handle, tt.userID, got, tt.want)
			}
		})
	}
}

func TestInfoSkipsEmptySignInAndSocialContext(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["sign_in"] = map[string]any{
		"account_created_ip": nil,
		"current_ip":         nil,
		"current_at":         nil,
		"last_ip":            nil,
		"last_at":            nil,
		"count":              0,
	}
	user["social"] = map[string]any{
		"twitter_user_id":          nil,
		"twitter_handle":           nil,
		"facebook_uid":             nil,
		"google_uid":               nil,
		"oauth_provider":           nil,
		"external_authentications": []map[string]any{},
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, unwanted := range []string{"Sign-in:", "Social:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output should skip empty %q block: %q", unwanted, out)
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

func TestInfoRendersActiveWatchedUser(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["active_watched_user"] = map[string]any{
		"id":                      "watch_123",
		"revenue_threshold_cents": 20000,
		"revenue_cents":           12500,
		"unpaid_balance_cents":    2500,
		"notes":                   "Check next independent buyers",
		"created_at":              "2026-05-01T10:00:00Z",
		"last_synced_at":          "2026-05-06T12:00:00Z",
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Watchlist: active",
		"id: watch_123",
		"revenue: $125.00 of $200.00",
		"unpaid balance: $25.00",
		"note: Check next independent buyers",
		"created: 2026-05-01T10:00:00Z",
		"last synced: 2026-05-06T12:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestInfoSuppressesDuplicateEmailLineWhenNameIsEmpty(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["name"] = ""

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "seller@example.com") {
		t.Fatalf("expected email in headline: %q", out)
	}
	if strings.Contains(out, "Email: seller@example.com") {
		t.Errorf("Email: line must be suppressed when headline already shows the email: %q", out)
	}
	if !strings.Contains(out, "Username: sellerone") {
		t.Errorf("downstream lines must still render: %q", out)
	}
}

func TestInfoSuppressesDuplicateUserIDLineWhenHeadlineIsUserID(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["name"] = ""
	user["email"] = ""

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Count(out, "2245593582708") != 1 {
		t.Fatalf("expected user_id to appear once, got: %q", out)
	}
	if strings.Contains(out, "User ID: 2245593582708") {
		t.Errorf("User ID line must be suppressed when headline already shows the user_id: %q", out)
	}
	if !strings.Contains(out, "Username: sellerone") {
		t.Errorf("downstream lines must still render: %q", out)
	}
}

func TestInfoFallsBackToUserRiskStateWhenStatusIsEmpty(t *testing.T) {
	payload := sampleInfoPayload()
	risk := payload["user"].(map[string]any)["risk_state"].(map[string]any)
	risk["status"] = ""
	risk["user_risk_state"] = "suspended_for_fraud"

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Risk: suspended_for_fraud") {
		t.Errorf("expected headline to fall back to user_risk_state: %q", out)
	}
	if strings.Contains(out, "user_risk_state: suspended_for_fraud") {
		t.Errorf("dedupe should suppress the indented line when it equals the headline: %q", out)
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})
	plainCmd := testutil.Command(newInfoCmd(), testutil.PlainOutput())
	plainCmd.SetArgs([]string{"--email", "seller@example.com"})
	plainOut := testutil.CaptureStdout(func() { testutil.MustExecute(t, plainCmd) })

	cols := strings.Split(strings.TrimSpace(plainOut), "\t")
	if len(cols) < 4 || cols[3] != "suspended_for_fraud" {
		t.Errorf("plain column 4 must fall back to user_risk_state when status is empty, got: %q", plainOut)
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

func TestInfoRendersStripeConnectedVerificationAndAdminLinks(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["stripe"] = map[string]any{
		"connected":                 true,
		"stripe_connect_account_id": "acct_123abc",
		"stripe_dashboard_url":      "https://dashboard.stripe.com/connect/accounts/acct_123abc",
		"verification": map[string]any{
			"charges_enabled":            true,
			"payouts_enabled":            false,
			"details_submitted":          true,
			"disabled_reason":            "requirements.past_due",
			"currently_due_count":        1,
			"past_due_count":             1,
			"pending_verification_count": 0,
		},
	}
	user["admin_links"] = map[string]any{
		"impersonate":      "https://app.example.com/admin/helper_actions/impersonate/u_abc",
		"admin_user":       "https://app.example.com/admin/users/123",
		"admin_purchases":  "https://app.example.com/admin/search/purchases?query=seller@example.com",
		"stripe_dashboard": "https://app.example.com/admin/helper_actions/stripe_dashboard/u_abc",
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Stripe: connected",
		"account: acct_123abc",
		"dashboard: https://dashboard.stripe.com/connect/accounts/acct_123abc",
		"charges enabled: true",
		"payouts enabled: false",
		"details submitted: true",
		"disabled reason: requirements.past_due",
		"currently due: 1",
		"past due: 1",
		"Admin links:",
		"impersonate: https://app.example.com/admin/helper_actions/impersonate/u_abc",
		"admin_user: https://app.example.com/admin/users/123",
		"admin_purchases: https://app.example.com/admin/search/purchases?query=seller@example.com",
		"stripe_dashboard: https://app.example.com/admin/helper_actions/stripe_dashboard/u_abc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "pending verification:") {
		t.Errorf("zero pending-verification count must be suppressed: %q", out)
	}
}

func TestInfoStripeConnectedPlainOutputCarriesAccountID(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["stripe"] = map[string]any{
		"connected":                 true,
		"stripe_connect_account_id": "acct_123abc",
		"stripe_dashboard_url":      "https://dashboard.stripe.com/connect/accounts/acct_123abc",
		"verification": map[string]any{
			"charges_enabled":   true,
			"payouts_enabled":   true,
			"details_submitted": true,
		},
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tSeller One\tsellerone\tCompliant\ttrue\tfalse\tfalse\t2026-05-15\t42\t$1,234.56\t2024-01-01T00:00:00Z\ttrue\tacct_123abc"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestInfoDegradesToVerificationErrorWhenStripeCallFailed(t *testing.T) {
	payload := sampleInfoPayload()
	user := payload["user"].(map[string]any)
	user["stripe"] = map[string]any{
		"connected":                 true,
		"stripe_connect_account_id": "acct_revoked",
		"stripe_dashboard_url":      "https://dashboard.stripe.com/connect/accounts/acct_revoked",
		"verification":              map[string]any{"error": "access revoked"},
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Stripe: connected",
		"account: acct_revoked",
		"verification error: access revoked",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "charges enabled:") {
		t.Errorf("degraded verification must not print Stripe flags: %q", out)
	}
}

func TestInfoOmitsStripeSectionWhenServerOmitsField(t *testing.T) {
	payload := sampleInfoPayload()
	if _, ok := payload["user"].(map[string]any)["stripe"]; ok {
		t.Fatal("sample payload must not include a stripe key for this test")
	}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Contains(out, "Stripe:") {
		t.Errorf("Stripe section must be omitted when the server omits the stripe field (e.g. CLI ahead of server): %q", out)
	}
	if !strings.Contains(out, "Sales: 42") {
		t.Errorf("downstream sections must still render: %q", out)
	}
}

func TestInfoReportsUnconnectedStripeWithoutInnerBlock(t *testing.T) {
	payload := sampleInfoPayload()
	payload["user"].(map[string]any)["stripe"] = map[string]any{"connected": false}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newInfoCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Stripe: not connected") {
		t.Errorf("expected unconnected Stripe headline: %q", out)
	}
	for _, unwanted := range []string{"charges enabled:", "dashboard:", "Admin links:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("unconnected Stripe / absent admin links must not render %q: %q", unwanted, out)
		}
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
