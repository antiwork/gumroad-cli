package purchases

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestViewUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{
				"id": "123", "email": "buyer@example.com", "seller_email": "seller@example.com",
				"link_name": "Course", "price_cents": 1200, "purchase_state": "successful",
				"created_at": "2026-04-24T12:00:00Z",
			},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/purchases/123" {
		t.Fatalf("got %s %s, want GET /internal/admin/purchases/123", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{"Course", "Purchase ID: 123", "Buyer: buyer@example.com", "Seller: seller@example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestViewSendsWithClustersAndRendersFraudContext(t *testing.T) {
	var gotClusters string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotClusters = r.URL.Query().Get("with_clusters")
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{
				"id":                    "123",
				"email":                 "buyer@example.com",
				"seller_email":          "legacy-seller@example.com",
				"seller":                map[string]any{"id": "usr_123", "email": "seller@example.com", "name": "Seller"},
				"product_name":          "Course",
				"formatted_total_price": "$12",
				"purchase_state":        "successful",
				"chargeback_date":       "2026-04-25T12:00:00Z",
				"charge_processor":      "stripe",
				"paypal_order_id":       "PAY-123",
				"ip_address":            "203.0.113.42",
				"ip_country":            "United States",
				"billing_country":       "Germany",
				"card_country":          "FR",
				"country_mismatches": map[string]any{
					"billing_vs_ip":   true,
					"billing_vs_card": true,
					"ip_vs_card":      true,
				},
				"card": map[string]any{
					"bin":          "424242",
					"type":         "visa",
					"visual":       "**** **** **** 4242",
					"expiry_month": 11,
					"expiry_year":  2030,
				},
				"dispute": map[string]any{
					"id":                          "disp_123",
					"state":                       "formalized",
					"reason":                      "fraudulent",
					"charge_processor_dispute_id": "dp_123",
					"formalized_at":               "2026-04-26T12:00:00Z",
				},
				"early_fraud_warning": map[string]any{
					"id":                   "1",
					"processor_id":         "issfr_123",
					"fraud_type":           "made_with_stolen_card",
					"charge_risk_level":    "highest",
					"actionable":           true,
					"resolution":           "unknown",
					"resolution_message":   "Issuer reported a stolen card",
					"processor_created_at": "2026-04-24T12:00:00Z",
				},
				"affiliate_credit": map[string]any{
					"amount_cents":      750,
					"fee_cents":         50,
					"basis_points":      1000,
					"affiliate_user_id": "usr_aff",
				},
				"clusters": map[string]any{
					"fingerprint_count": 2,
					"browser_count":     1,
					"ip_count":          3,
				},
			},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123", "--with-clusters"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotClusters != "true" {
		t.Fatalf("got with_clusters=%q, want true", gotClusters)
	}
	for _, want := range []string{
		"Seller: seller@example.com",
		"Risk:",
		"Country mismatches: billing_vs_ip, billing_vs_card, ip_vs_card",
		"Card: **** **** **** 4242, visa, BIN 424242, country FR, exp 11/2030",
		"Chargeback: 2026-04-25T12:00:00Z",
		"Dispute: formalized, fraudulent, dp_123, formalized 2026-04-26T12:00:00Z",
		"Early fraud warning: made_with_stolen_card, risk highest, actionable, resolution unknown: Issuer reported a stolen card, issfr_123",
		"IP: 203.0.113.42 (United States)",
		"Processor: stripe",
		"PayPal order: PAY-123",
		"Affiliate credit: 750 cents (fee 50 cents, 1000 bps, affiliate usr_aff)",
		"Clusters: fingerprint 2, browser 1, IP 3",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "legacy-seller@example.com") {
		t.Fatalf("output must prefer seller.email over legacy seller_email: %q", out)
	}
}

func TestViewOmitsSparseFraudContext(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{
				"id":                  "123",
				"card":                map[string]any{"visual": "**** **** **** 4242", "expiry_year": 2030},
				"dispute":             map[string]any{},
				"early_fraud_warning": map[string]any{},
				"affiliate_credit":    map[string]any{},
				"country_mismatches":  map[string]any{"billing_vs_ip": false, "billing_vs_card": false, "ip_vs_card": false},
			},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Card: **** **** **** 4242") {
		t.Fatalf("expected card visual in output: %q", out)
	}
	for _, unwanted := range []string{"exp ", "Dispute: \n", "Early fraud warning: \n", "Affiliate credit: 0 cents"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("output must omit sparse fraud detail %q: %q", unwanted, out)
		}
	}
}

func TestViewOmitsClustersUnlessRequested(t *testing.T) {
	var gotClusters string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotClusters = r.URL.Query().Get("with_clusters")
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{
				"id":       "123",
				"clusters": map[string]any{"fingerprint_count": 2, "browser_count": 1, "ip_count": 3},
			},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotClusters != "" {
		t.Fatalf("with_clusters must be omitted by default, got %q", gotClusters)
	}
	if strings.Contains(out, "Clusters:") {
		t.Fatalf("clusters must be hidden unless --with-clusters is set: %q", out)
	}
}

func TestViewJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"id": "123", "email": "buyer@example.com"},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Purchase map[string]any `json:"purchase"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.Purchase["id"] != "123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestViewPlainOutputUsesFallbackFields(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{
				"id":             "123",
				"email":          "buyer@example.com",
				"seller_email":   "seller@example.com",
				"product_id":     "prod_123",
				"price_cents":    5000,
				"purchase_state": "successful",
				"created_at":     "2026-04-24T12:00:00Z",
				"receipt_url":    "https://gumroad.com/receipts/123",
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "123\tbuyer@example.com\tseller@example.com\tprod_123\t5000 cents\tsuccessful\t2026-04-24T12:00:00Z\thttps://gumroad.com/receipts/123"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestViewHumanOutputOmitsEmptyOptionalFields(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"id": "123"},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "123" {
		t.Fatalf("unexpected human output: %q", out)
	}
}

func TestViewHumanOutputAvoidsDuplicateIDWithAmountFallback(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"id": "123", "formatted_total_price": "$12"},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "123  $12" {
		t.Fatalf("unexpected human output: %q", out)
	}
}

func TestViewHumanOutputUsesFormattedTotalAndReceipt(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{
				"id":                    "123",
				"product_name":          "Course",
				"formatted_total_price": "$12",
				"purchase_state":        "successful",
				"refund_status":         "partially_refunded",
				"receipt_url":           "https://gumroad.com/receipts/123",
			},
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Course  $12",
		"Status: successful, partially_refunded",
		"Receipt: https://gumroad.com/receipts/123",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestNewPurchasesCmdWiresSubcommands(t *testing.T) {
	cmd := NewPurchasesCmd()
	if cmd.Use != "purchases" {
		t.Fatalf("Use = %q, want purchases", cmd.Use)
	}

	got := cmd.Commands()
	want := []string{
		"view <purchase-id>",
		"search",
		"refund <purchase-id>",
		"refund-taxes <purchase-id>",
		"refund-for-fraud <purchase-id>",
		"cancel-subscription <purchase-id>",
		"block-buyer <purchase-id>",
		"unblock-buyer <purchase-id>",
		"resend-receipt <purchase-id>",
		"resend-all-receipts",
		"reassign",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d subcommands, got %d: %#v", len(want), len(got), got)
	}

	names := map[string]bool{}
	for _, sub := range got {
		names[sub.Use] = true
	}
	for _, name := range want {
		if !names[name] {
			t.Errorf("missing subcommand %q in %v", name, names)
		}
	}
}
