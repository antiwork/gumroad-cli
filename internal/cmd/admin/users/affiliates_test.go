package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestAffiliatesUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotEmail, gotDirection, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		gotDirection = r.URL.Query().Get("direction")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"user_id":   "seller_123",
			"direction": "granted",
			"affiliates": []map[string]any{
				affiliateFixture(),
			},
			"pagination": map[string]any{"next": nil, "limit": 20},
		})
	})

	cmd := testutil.Command(newAffiliatesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--direction", "granted"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/affiliates" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/affiliates", gotMethod, gotPath)
	}
	if gotEmail != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", gotEmail)
	}
	if gotDirection != "granted" {
		t.Fatalf("got direction %q, want granted", gotDirection)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{
		"1 granted affiliate relationship(s) for seller@example.com",
		"User ID: seller_123",
		"DirectAffiliate",
		"affiliate@example.com / Affiliate User / user_456",
		"1500",
		"Starter Pack (prod_123)",
		"true",
		"2026-05-01T12:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestAffiliatesPassesUserIDLimitAndCursor(t *testing.T) {
	var gotQuery string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"affiliates":  []any{},
			"pagination":  map[string]any{"next": nil, "limit": 50},
			"success":     true,
			"user_id":     "user_123",
			"unused_note": "ignored",
		})
	})

	cmd := testutil.Command(newAffiliatesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123", "--direction", "received", "--limit", "50", "--cursor", "cur-1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"user_id=user_123", "direction=received", "limit=50", "cursor=cur-1"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %q: %q", want, gotQuery)
		}
	}
	if !strings.Contains(out, "No received affiliate relationships found for user_123.") {
		t.Fatalf("unexpected empty output: %q", out)
	}
}

func TestAffiliatesShowsNextCursorFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"direction": "granted",
			"affiliates": []map[string]any{
				affiliateFixture(),
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 1},
		})
	})

	cmd := testutil.Command(newAffiliatesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "seller_123", "--direction", "granted"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "More results: --cursor cur-next") {
		t.Fatalf("expected next-cursor footer, got: %q", out)
	}
}

func TestAffiliatesRequiresEmailOrUserID(t *testing.T) {
	cmd := newAffiliatesCmd()
	cmd.SetArgs([]string{"--direction", "granted"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email or --user-id") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestAffiliatesRequiresDirection(t *testing.T) {
	cmd := newAffiliatesCmd()
	cmd.SetArgs([]string{"--user-id", "user_123"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --direction") {
		t.Fatalf("expected missing direction error, got %v", err)
	}
}

func TestAffiliatesRejectsInvalidDirection(t *testing.T) {
	cmd := newAffiliatesCmd()
	cmd.SetArgs([]string{"--user-id", "user_123", "--direction", "both"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--direction must be one of: granted, received") {
		t.Fatalf("expected direction validation error, got %v", err)
	}
}

func TestAffiliatesRejectsInvalidLimit(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		cmd := newAffiliatesCmd()
		cmd.SetArgs([]string{"--user-id", "user_123", "--direction", "granted", "--limit", "0"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
			t.Fatalf("expected zero limit error, got %v", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		cmd := newAffiliatesCmd()
		cmd.SetArgs([]string{"--user-id", "user_123", "--direction", "granted", "--limit", "101"})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--limit must be 100 or less") {
			t.Fatalf("expected max limit error, got %v", err)
		}
	})
}

func TestAffiliatesJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":   true,
			"user_id":   "seller_123",
			"direction": "granted",
			"affiliates": []map[string]any{
				affiliateFixture(),
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newAffiliatesCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "seller_123", "--direction", "granted"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool             `json:"success"`
		UserID     string           `json:"user_id"`
		Direction  string           `json:"direction"`
		Affiliates []map[string]any `json:"affiliates"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "seller_123" || resp.Direction != "granted" {
		t.Fatalf("unexpected JSON envelope: %s", out)
	}
	if len(resp.Affiliates) != 1 || resp.Affiliates[0]["id"] != "aff_123" {
		t.Fatalf("unexpected JSON affiliates: %s", out)
	}
	if resp.Pagination["next"] != "cur-next" {
		t.Fatalf("unexpected pagination: %s", out)
	}
}

func TestAffiliatesPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"direction": "received",
			"affiliates": []map[string]any{
				affiliateFixture(),
				{
					"id":                     "aff_all",
					"type":                   "Collaborator",
					"direction":              "received",
					"counterparty":           map[string]any{"email": "seller@example.com"},
					"affiliate_basis_points": 2000,
					"apply_to_all_products":  true,
					"alive":                  false,
					"created_at":             "2026-04-30T12:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newAffiliatesCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "affiliate@example.com", "--direction", "received"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := strings.Join([]string{
		"aff_123\tDirectAffiliate\taffiliate@example.com / Affiliate User / user_456\t1500\tStarter Pack (prod_123)\ttrue\t2026-05-01T12:00:00Z",
		"aff_all\tCollaborator\tseller@example.com\t2000\tall products\tfalse\t2026-04-30T12:00:00Z",
	}, "\n")
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestAffiliateProductLabelFallsBackToID(t *testing.T) {
	if got := affiliateProductLabel(affiliateProduct{ID: "prod_123"}); got != "prod_123" {
		t.Fatalf("got %q, want prod_123", got)
	}
}

func affiliateFixture() map[string]any {
	return map[string]any{
		"id":                     "aff_123",
		"type":                   "DirectAffiliate",
		"direction":              "granted",
		"counterparty":           map[string]any{"id": "user_456", "email": "affiliate@example.com", "name": "Affiliate User"},
		"affiliate_basis_points": 1500,
		"destination_url":        "https://example.com/aff",
		"apply_to_all_products":  false,
		"alive":                  true,
		"deleted_at":             nil,
		"created_at":             "2026-05-01T12:00:00Z",
		"products": []map[string]any{
			{
				"id":              "prod_123",
				"name":            "Starter Pack",
				"basis_points":    1500,
				"destination_url": "https://example.com/product",
			},
		},
	}
}
