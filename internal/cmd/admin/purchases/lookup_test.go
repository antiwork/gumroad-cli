package purchases

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestLookupUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotFingerprint, gotBrowserGUID, gotIPAddress, gotLimit, gotCursor string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotFingerprint = r.URL.Query().Get("stripe_fingerprint")
		gotBrowserGUID = r.URL.Query().Get("browser_guid")
		gotIPAddress = r.URL.Query().Get("ip_address")
		gotLimit = r.URL.Query().Get("limit")
		gotCursor = r.URL.Query().Get("cursor")
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"lookup":  map[string]any{"field": "stripe_fingerprint", "value": "fp_shared"},
			"purchases": []map[string]any{
				lookupPurchaseFixture(),
			},
			"pagination": map[string]any{"next": nil, "limit": 25},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--stripe-fingerprint", " fp_shared ", "--limit", "25", "--cursor", "cur-1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/purchases/lookup" {
		t.Fatalf("got %s %s, want GET /internal/admin/purchases/lookup", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotFingerprint != "fp_shared" {
		t.Fatalf("got stripe_fingerprint %q, want fp_shared", gotFingerprint)
	}
	if gotBrowserGUID != "" || gotIPAddress != "" {
		t.Fatalf("unexpected extra lookup params: browser_guid=%q ip_address=%q", gotBrowserGUID, gotIPAddress)
	}
	if gotLimit != "25" || gotCursor != "cur-1" {
		t.Fatalf("got limit=%q cursor=%q, want 25 cur-1", gotLimit, gotCursor)
	}
	for _, want := range []string{
		"1 purchase(s) for stripe_fingerprint=fp_shared",
		"SELLER",
		"BUYER",
		"seller@example.com / Seller User / seller_123",
		"buyer@example.com",
		"Course",
		"$12",
		"successful",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestLookupSupportsBrowserGUIDAndIPAddress(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantField string
		wantValue string
	}{
		{
			name:      "browser GUID",
			args:      []string{"--browser-guid", "bguid_abc"},
			wantField: "browser_guid",
			wantValue: "bguid_abc",
		},
		{
			name:      "IP address",
			args:      []string{"--ip-address", "203.0.113.7"},
			wantField: "ip_address",
			wantValue: "203.0.113.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotValue string
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				gotValue = r.URL.Query().Get(tt.wantField)
				testutil.JSON(t, w, map[string]any{
					"lookup":     map[string]any{"field": tt.wantField, "value": tt.wantValue},
					"purchases":  []map[string]any{},
					"pagination": map[string]any{"next": nil, "limit": 20},
				})
			})

			cmd := testutil.Command(newLookupCmd(), testutil.Quiet(false))
			cmd.SetArgs(tt.args)
			testutil.MustExecute(t, cmd)

			if gotValue != tt.wantValue {
				t.Fatalf("got %s=%q, want %q", tt.wantField, gotValue, tt.wantValue)
			}
		})
	}
}

func TestLookupShowsEmptyResultAndNextCursorFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":    true,
			"lookup":     map[string]any{"field": "ip_address", "value": "203.0.113.7"},
			"purchases":  []map[string]any{},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--ip-address", "203.0.113.7"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"No purchases found for ip_address=203.0.113.7.",
		"More results: --cursor cur-next",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestLookupPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"lookup": map[string]any{"field": "stripe_fingerprint", "value": "fp_shared"},
			"purchases": []map[string]any{
				lookupPurchaseFixture(),
				{
					"id":                    "456",
					"email":                 "second-buyer@example.com",
					"seller_email":          "legacy-seller@example.com",
					"link_name":             "Bundle",
					"formatted_total_price": "$20",
					"purchase_state":        "refunded",
					"created_at":            "2026-05-11T12:00:00Z",
				},
			},
			"pagination": map[string]any{"next": nil, "limit": 20},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--stripe-fingerprint", "fp_shared"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := strings.Join([]string{
		"123\tbuyer@example.com\tseller@example.com / Seller User / seller_123\tCourse\t$12\tsuccessful\t2026-05-12T12:00:00Z",
		"456\tsecond-buyer@example.com\tlegacy-seller@example.com\tBundle\t$20\trefunded\t2026-05-11T12:00:00Z",
	}, "\n")
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestLookupSellerLabelSkipsDuplicateName(t *testing.T) {
	p := purchase{Seller: &purchaseSeller{
		ID:    "seller_123",
		Email: "seller@example.com",
		Name:  "seller@example.com",
	}}

	if got, want := sellerLabel(p), "seller@example.com / seller_123"; got != want {
		t.Fatalf("sellerLabel() = %q, want %q", got, want)
	}
}

func TestLookupJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"lookup":  map[string]any{"field": "browser_guid", "value": "bguid_abc"},
			"purchases": []map[string]any{
				lookupPurchaseFixture(),
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--browser-guid", "bguid_abc"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool             `json:"success"`
		Lookup     map[string]any   `json:"lookup"`
		Purchases  []map[string]any `json:"purchases"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Lookup["field"] != "browser_guid" || resp.Lookup["value"] != "bguid_abc" {
		t.Fatalf("unexpected JSON lookup envelope: %s", out)
	}
	if len(resp.Purchases) != 1 || resp.Purchases[0]["id"] != "123" {
		t.Fatalf("unexpected JSON purchases: %s", out)
	}
	if resp.Pagination["next"] != "cur-next" {
		t.Fatalf("unexpected JSON pagination: %s", out)
	}
}

func TestLookupRequiresExactlyOneSignal(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing signal",
			args: []string{},
			want: "exactly one of --stripe-fingerprint, --browser-guid, or --ip-address must be provided",
		},
		{
			name: "multiple signals",
			args: []string{"--stripe-fingerprint", "fp_abc", "--ip-address", "203.0.113.7"},
			want: "exactly one of --stripe-fingerprint, --browser-guid, or --ip-address must be provided",
		},
		{
			name: "empty signal",
			args: []string{"--stripe-fingerprint", "  "},
			want: "--stripe-fingerprint cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("must not request when signal validation fails")
			})

			cmd := testutil.Command(newLookupCmd())
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestLookupRejectsInvalidLimit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not request when --limit is invalid")
	})

	cmd := testutil.Command(newLookupCmd())
	cmd.SetArgs([]string{"--ip-address", "203.0.113.7", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected zero-limit error, got: %v", err)
	}
}

func lookupPurchaseFixture() map[string]any {
	return map[string]any{
		"id":                    "123",
		"email":                 "buyer@example.com",
		"seller_email":          "seller@example.com",
		"seller":                map[string]any{"id": "seller_123", "email": "seller@example.com", "name": "Seller User"},
		"product_name":          "Course",
		"formatted_total_price": "$12",
		"purchase_state":        "successful",
		"created_at":            "2026-05-12T12:00:00Z",
	}
}
