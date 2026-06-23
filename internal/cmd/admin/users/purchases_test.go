package users

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUserPurchasesUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotQuery url.Values
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"user_id": "user_123",
			"purchases": []map[string]any{
				userPurchaseFixture(),
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 50},
		})
	})

	cmd := testutil.Command(newPurchasesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{
		"--email", "buyer@example.com",
		"--status", "successful",
		"--status", "failed",
		"--start-at", "2026-01-01T00:00:00Z",
		"--end-at", "2026-05-01T00:00:00Z",
		"--stripe-fingerprint", "fp_shared",
		"--ip-address", "203.0.113.7",
		"--chargedback",
		"--has-early-fraud-warning=false",
		"--has-affiliate",
		"--limit", "50",
		"--cursor", "cur-1",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/purchases" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/purchases", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for key, want := range map[string]string{
		"email":                   "buyer@example.com",
		"status":                  "successful,failed",
		"start_at":                "2026-01-01T00:00:00Z",
		"end_at":                  "2026-05-01T00:00:00Z",
		"stripe_fingerprint":      "fp_shared",
		"ip_address":              "203.0.113.7",
		"chargedback":             "true",
		"has_early_fraud_warning": "false",
		"has_affiliate":           "true",
		"limit":                   "50",
		"cursor":                  "cur-1",
	} {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q; full query %s", key, got, want, gotQuery.Encode())
		}
	}
	for _, want := range []string{
		"1 purchase(s) for buyer@example.com",
		"User ID: user_123",
		"seller@example.com",
		"Investigation guide",
		"$12.34",
		"successful, refunded",
		"CB,EFW,COUNTRY",
		"2026-05-01T12:00:00Z",
		"More results: --cursor cur-next",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestUserPurchasesPassesUserIDAndFalseBooleanFilters(t *testing.T) {
	var gotQuery url.Values
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"user_id":    "user_123",
			"purchases":  []any{},
			"pagination": map[string]any{"next": nil, "limit": 20},
		})
	})

	cmd := testutil.Command(newPurchasesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123", "--chargedback=false", "--has-early-fraud-warning=false", "--has-affiliate=false"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for key, want := range map[string]string{
		"user_id":                 "user_123",
		"chargedback":             "false",
		"has_early_fraud_warning": "false",
		"has_affiliate":           "false",
	} {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q; full query %s", key, got, want, gotQuery.Encode())
		}
	}
}

func TestUserPurchasesOmitsUnsetBooleanFilters(t *testing.T) {
	var gotQuery url.Values
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"user_id":    "user_123",
			"purchases":  []any{},
			"pagination": map[string]any{"next": nil, "limit": 20},
		})
	})

	cmd := testutil.Command(newPurchasesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, key := range []string{"chargedback", "has_early_fraud_warning", "has_affiliate"} {
		if gotQuery.Has(key) {
			t.Fatalf("query unexpectedly included %s: %s", key, gotQuery.Encode())
		}
	}
}

func TestUserPurchasesEmptyResultShowsFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id":    "user_123",
			"purchases":  []any{},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newPurchasesCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"No purchases found for user_123.",
		"More results: --cursor cur-next",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestUserPurchasesJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":   true,
			"user_id":   "user_123",
			"purchases": []map[string]any{userPurchaseFixture()},
			"pagination": map[string]any{
				"next":  "cur-next",
				"limit": 20,
			},
		})
	})

	cmd := testutil.Command(newPurchasesCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool             `json:"success"`
		UserID     string           `json:"user_id"`
		Purchases  []map[string]any `json:"purchases"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "user_123" {
		t.Fatalf("unexpected JSON envelope: %s", out)
	}
	if len(resp.Purchases) != 1 || resp.Purchases[0]["id"] != "purchase_123" {
		t.Fatalf("unexpected purchases JSON: %s", out)
	}
	if resp.Pagination["next"] != "cur-next" {
		t.Fatalf("unexpected pagination JSON: %s", out)
	}
}

func TestUserPurchasesPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{
				userPurchaseFixture(),
				{
					"id":                  "purchase_456",
					"email":               "buyer@example.com",
					"seller_email":        "legacy-seller@example.com",
					"product_name":        "Second product",
					"price_cents":         900,
					"currency_type":       "usd",
					"purchase_state":      "failed",
					"created_at":          "2026-04-30T12:00:00Z",
					"country_mismatches":  map[string]any{"billing_vs_ip": false, "billing_vs_card": false, "ip_vs_card": false},
					"early_fraud_warning": nil,
				},
			},
		})
	})

	cmd := testutil.Command(newPurchasesCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := strings.Join([]string{
		"purchase_123\tseller@example.com\tInvestigation guide\t$12.34\tsuccessful, refunded\tCB,EFW,COUNTRY\t2026-05-01T12:00:00Z",
		"purchase_456\tlegacy-seller@example.com\tSecond product\t900 cents\tfailed\t\t2026-04-30T12:00:00Z",
	}, "\n")
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestUserPurchasesRequiresEmailOrUserID(t *testing.T) {
	cmd := newPurchasesCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email, --user-id, or --username") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestUserPurchasesRejectsInvalidLimit(t *testing.T) {
	cmd := newPurchasesCmd()
	cmd.SetArgs([]string{"--user-id", "user_123", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected limit validation error, got %v", err)
	}
}

func TestUserPurchasesRejectsEmptyStatus(t *testing.T) {
	cmd := newPurchasesCmd()
	cmd.SetArgs([]string{"--user-id", "user_123", "--status", ""})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--status cannot be empty") {
		t.Fatalf("expected empty status validation error, got %v", err)
	}
}

func userPurchaseFixture() map[string]any {
	return map[string]any{
		"id":                                  "purchase_123",
		"email":                               "buyer@example.com",
		"seller_email":                        "legacy-seller@example.com",
		"seller":                              map[string]any{"id": "seller_123", "email": "seller@example.com", "name": "Seller User"},
		"product_name":                        "Investigation guide",
		"link_name":                           "investigation-guide",
		"product_id":                          "prod_123",
		"formatted_total_price":               "$12.34",
		"price_cents":                         1234,
		"currency_type":                       "usd",
		"purchase_state":                      "successful",
		"refund_status":                       "refunded",
		"chargeback_date":                     "2026-05-02T12:00:00Z",
		"created_at":                          "2026-05-01T12:00:00Z",
		"country_mismatches":                  map[string]any{"billing_vs_ip": true, "billing_vs_card": false, "ip_vs_card": false},
		"early_fraud_warning":                 map[string]any{"id": "efw_123"},
		"amount_refundable_cents_in_currency": 0,
	}
}
