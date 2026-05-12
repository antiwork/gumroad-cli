package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRelatedUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotEmail, gotSignals, gotLimit, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		gotSignals = r.URL.Query().Get("signals")
		gotLimit = r.URL.Query().Get("limit")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"user_id":           "target_123",
			"signals_evaluated": []string{"ip", "payment_address"},
			"per_signal_limit":  10,
			"related_users": []map[string]any{
				relatedUserFixture(),
			},
			"truncated": map[string]any{"ip": false, "payment_address": false},
		})
	})

	cmd := testutil.Command(newRelatedCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "target@example.com", "--signal", "ip", "--signal", "payment_address", "--limit", "10"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/related" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/related", gotMethod, gotPath)
	}
	if gotEmail != "target@example.com" {
		t.Fatalf("got email %q, want target@example.com", gotEmail)
	}
	if gotSignals != "ip,payment_address" {
		t.Fatalf("got signals %q, want ip,payment_address", gotSignals)
	}
	if gotLimit != "10" {
		t.Fatalf("got limit %q, want 10", gotLimit)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{
		"1 related user(s) for target@example.com",
		"User ID: target_123",
		"Signals evaluated: ip, payment_address",
		"Per-signal limit: 10",
		"rel_123",
		"related@example.com",
		"Related User",
		"Suspended for fraud",
		"ip:1.2.3.4 (account_created_ip, current_sign_in_ip)",
		"payment_address:shared@example.com",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRelatedDefaultOmitsSignalsAndLimit(t *testing.T) {
	var gotSignals, gotLimit, gotUserID string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotSignals = r.URL.Query().Get("signals")
		gotLimit = r.URL.Query().Get("limit")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, map[string]any{
			"user_id":           "user_123",
			"signals_evaluated": []string{},
			"per_signal_limit":  50,
			"related_users":     []any{},
			"truncated":         map[string]any{},
		})
	})

	cmd := testutil.Command(newRelatedCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotUserID != "user_123" {
		t.Fatalf("got user_id %q, want user_123", gotUserID)
	}
	if gotSignals != "" {
		t.Fatalf("default command must omit signals, got %q", gotSignals)
	}
	if gotLimit != "" {
		t.Fatalf("default command must omit limit, got %q", gotLimit)
	}
	for _, want := range []string{
		"0 related user(s) for user_123",
		"Signals evaluated: none",
		"Per-signal limit: 50",
		"No related users found for user_123.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRelatedRequiresEmailOrUserID(t *testing.T) {
	cmd := newRelatedCmd()
	cmd.SetArgs([]string{"--signal", "ip"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email or --user-id") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestRelatedRejectsInvalidSignal(t *testing.T) {
	cmd := newRelatedCmd()
	cmd.SetArgs([]string{"--user-id", "user_123", "--signal", "ip,payment_address"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--signal must be one of: ip, payment_address, card_fingerprint") {
		t.Fatalf("expected signal validation error, got %v", err)
	}
}

func TestRelatedRejectsInvalidLimit(t *testing.T) {
	cmd := newRelatedCmd()
	cmd.SetArgs([]string{"--user-id", "user_123", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected limit validation error, got %v", err)
	}
}

func TestRelatedShowsTruncationFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"signals_evaluated": []string{"ip", "payment_address"},
			"per_signal_limit":  1,
			"related_users": []map[string]any{
				relatedUserFixture(),
			},
			"truncated": map[string]any{"payment_address": true, "ip": true},
		})
	})

	cmd := testutil.Command(newRelatedCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "target_123", "--signal", "ip", "--signal", "payment_address", "--limit", "1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Warning: results capped for signals: ip, payment_address.") {
		t.Fatalf("expected truncation warning, got: %q", out)
	}
}

func TestRelatedJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":           true,
			"user_id":           "target_123",
			"signals_evaluated": []string{"card_fingerprint"},
			"per_signal_limit":  50,
			"related_users": []map[string]any{
				relatedUserFixture(),
			},
			"truncated": map[string]any{"card_fingerprint": false},
		})
	})

	cmd := testutil.Command(newRelatedCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "target_123", "--signal", "card_fingerprint"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success          bool             `json:"success"`
		UserID           string           `json:"user_id"`
		SignalsEvaluated []string         `json:"signals_evaluated"`
		RelatedUsers     []map[string]any `json:"related_users"`
		Truncated        map[string]bool  `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "target_123" {
		t.Fatalf("unexpected JSON envelope: %s", out)
	}
	if len(resp.SignalsEvaluated) != 1 || resp.SignalsEvaluated[0] != "card_fingerprint" {
		t.Fatalf("unexpected JSON signals: %s", out)
	}
	if len(resp.RelatedUsers) != 1 || resp.RelatedUsers[0]["id"] != "rel_123" {
		t.Fatalf("unexpected JSON related users: %s", out)
	}
	if resp.Truncated["card_fingerprint"] {
		t.Fatalf("unexpected JSON truncated map: %s", out)
	}
}

func TestRelatedPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"signals_evaluated": []string{"ip", "card_fingerprint"},
			"per_signal_limit":  50,
			"related_users": []map[string]any{
				relatedUserFixture(),
				{
					"id":    "rel_456",
					"email": "card@example.com",
					"name":  "",
					"risk_state": map[string]any{
						"user_risk_state": "compliant",
					},
					"relations": []map[string]any{
						{
							"signal":       "card_fingerprint",
							"shared_value": nil,
						},
					},
				},
			},
			"truncated": map[string]any{"ip": true},
		})
	})

	cmd := testutil.Command(newRelatedCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "target@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := strings.Join([]string{
		"rel_123\trelated@example.com\tRelated User\tSuspended for fraud\tip:1.2.3.4 (account_created_ip, current_sign_in_ip), payment_address:shared@example.com",
		"rel_456\tcard@example.com\t\tcompliant\tcard_fingerprint",
	}, "\n")
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestRelatedRelationLabelWithoutSharedValueIncludesVia(t *testing.T) {
	got := relatedRelationLabel(relatedRelation{Signal: "card_fingerprint", Via: []string{"credit_card"}})
	if got != "card_fingerprint (credit_card)" {
		t.Fatalf("got %q, want card_fingerprint (credit_card)", got)
	}
}

func relatedUserFixture() map[string]any {
	return map[string]any{
		"id":    "rel_123",
		"email": "related@example.com",
		"name":  "Related User",
		"risk_state": map[string]any{
			"status":          "Suspended for fraud",
			"user_risk_state": "suspended_for_fraud",
		},
		"relations": []map[string]any{
			{
				"signal":       "ip",
				"shared_value": "1.2.3.4",
				"via":          []string{"account_created_ip", "current_sign_in_ip"},
			},
			{
				"signal":       "payment_address",
				"shared_value": "shared@example.com",
			},
		},
	}
}
