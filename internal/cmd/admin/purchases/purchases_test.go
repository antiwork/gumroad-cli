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

func TestNewPurchasesCmdWiresView(t *testing.T) {
	cmd := NewPurchasesCmd()
	if cmd.Use != "purchases" {
		t.Fatalf("Use = %q, want purchases", cmd.Use)
	}
	if got := cmd.Commands(); len(got) != 1 || got[0].Use != "view <purchase-id>" {
		t.Fatalf("unexpected subcommands: %#v", got)
	}
}
