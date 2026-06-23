package users

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRadarUsesInternalAdminEndpointAndRendersHumanOutput(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotQuery url.Values

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, radarPayload())
	})

	cmd := testutil.Command(newRadarCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--limit", "50", "--cursor", "cur-1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/radar_stats" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/radar_stats", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for key, want := range map[string]string{
		"email":  "seller@example.com",
		"limit":  "50",
		"cursor": "cur-1",
	} {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q; full query %s", key, got, want, gotQuery.Encode())
		}
	}

	for _, want := range []string{
		"Radar stats for seller@example.com",
		"User ID: user_123",
		"Successful purchases: 3",
		"Early fraud warnings: 3",
		"EFW by fraud type: made_with_stolen_card=2, misc=1",
		"Elevated risk EFWs: 2",
		"Highest risk EFWs: 1",
		"Disputes: 1",
		"Dispute rate: 33.33%",
		"Recent EFWs:",
		"purchase_123",
		"made_with_stolen_card",
		"highest",
		"unknown",
		"2026-05-15T12:00:00Z",
		"CH-charge_123",
		"resolved_ignored",
		"More results: --cursor cur-next",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRadarPassesUserID(t *testing.T) {
	var gotEmail, gotUserID string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, emptyRadarPayload())
	})

	cmd := testutil.Command(newRadarCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotEmail != "" || gotUserID != "user_123" {
		t.Fatalf("got email=%q user_id=%q, want only user_id", gotEmail, gotUserID)
	}
	for _, want := range []string{
		"Radar stats for user_123",
		"Successful purchases: 0",
		"EFW by fraud type: (none)",
		"Recent EFWs: none",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRadarJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, radarPayload())
	})

	cmd := testutil.Command(newRadarCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool             `json:"success"`
		UserID     string           `json:"user_id"`
		RadarStats map[string]any   `json:"radar_stats"`
		RecentEFWs []map[string]any `json:"recent_efws"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "user_123" {
		t.Fatalf("unexpected JSON envelope: %s", out)
	}
	if resp.RadarStats["efw_count"] != float64(3) {
		t.Fatalf("unexpected radar_stats JSON: %s", out)
	}
	if len(resp.RecentEFWs) != 2 || resp.RecentEFWs[0]["purchase_id"] != "purchase_123" {
		t.Fatalf("unexpected recent_efws JSON: %s", out)
	}
	if resp.Pagination["next"] != "cur-next" {
		t.Fatalf("unexpected pagination JSON: %s", out)
	}
}

func TestRadarPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, radarPayload())
	})

	cmd := testutil.Command(newRadarCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tuser_123\t3\t3\tmade_with_stolen_card=2, misc=1\t2\t1\t1\t33.33"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestRadarPlainOutputWithoutRecentEFWs(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, emptyRadarPayload())
	})

	cmd := testutil.Command(newRadarCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "user_123\tuser_123\t0\t0\t(none)\t0\t0\t0\t0"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected empty plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestRadarRequiresEmailOrUserID(t *testing.T) {
	cmd := newRadarCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email, --user-id, or --username") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestRadarRejectsInvalidLimit(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		cmd := newRadarCmd()
		cmd.SetArgs([]string{"--user-id", "user_123", "--limit", "0"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
			t.Fatalf("expected zero limit error, got %v", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		cmd := newRadarCmd()
		cmd.SetArgs([]string{"--user-id", "user_123", "--limit", "101"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--limit must be 100 or less") {
			t.Fatalf("expected max limit error, got %v", err)
		}
	})
}

func radarPayload() map[string]any {
	payload := emptyRadarPayload()
	payload["radar_stats"] = map[string]any{
		"successful_purchases":   3,
		"efw_count":              3,
		"efw_by_fraud_type":      map[string]any{"made_with_stolen_card": 2, "misc": 1},
		"efw_with_elevated_risk": 2,
		"efw_with_highest_risk":  1,
		"dispute_count":          1,
		"dispute_rate":           33.33,
	}
	payload["recent_efws"] = []map[string]any{
		{
			"purchase_id":       "purchase_123",
			"fraud_type":        "made_with_stolen_card",
			"charge_risk_level": "highest",
			"resolution":        "unknown",
			"created_at":        "2026-05-15T12:00:00Z",
		},
		{
			"purchase_id":       "CH-charge_123",
			"fraud_type":        "misc",
			"charge_risk_level": "elevated",
			"resolution":        "resolved_ignored",
			"created_at":        "2026-05-15T11:00:00Z",
		},
	}
	payload["pagination"] = map[string]any{"next": "cur-next", "limit": 50}
	return payload
}

func emptyRadarPayload() map[string]any {
	return map[string]any{
		"success": true,
		"user_id": "user_123",
		"radar_stats": map[string]any{
			"successful_purchases":   0,
			"efw_count":              0,
			"efw_by_fraud_type":      map[string]any{},
			"efw_with_elevated_risk": 0,
			"efw_with_highest_risk":  0,
			"dispute_count":          0,
			"dispute_rate":           0.0,
		},
		"recent_efws": []any{},
		"pagination":  map[string]any{"next": nil, "limit": 20},
	}
}
