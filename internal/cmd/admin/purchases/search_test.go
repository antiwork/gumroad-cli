package purchases

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestSearch_RequiresEmail(t *testing.T) {
	cmd := newSearchCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearch_SendsEmailInQuery(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotEmail string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotEmail = r.URL.Query().Get("email")
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{
				{
					"id":                    "1",
					"email":                 "buyer@example.com",
					"product_name":          "Course",
					"formatted_total_price": "$12",
					"purchase_state":        "successful",
					"created_at":            "2026-04-24T12:00:00Z",
				},
			},
			"count":    1,
			"limit":    25,
			"has_more": false,
		})
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/purchases/search" {
		t.Fatalf("got %s %s, want GET /internal/admin/purchases/search", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotEmail != "buyer@example.com" {
		t.Errorf("got email %q, want buyer@example.com", gotEmail)
	}
	for _, want := range []string{"1 purchase(s) for buyer@example.com", "Course", "Buyer: buyer@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: %q", want, out)
		}
	}
}

func TestSearch_OmitsLimitWhenNotSet(t *testing.T) {
	var gotLimit string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{},
			"count":     0,
			"limit":     25,
			"has_more":  false,
		})
	})

	cmd := testutil.Command(newSearchCmd())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	testutil.MustExecute(t, cmd)

	if gotLimit != "" {
		t.Errorf("limit must be omitted when not set, got %q", gotLimit)
	}
}

func TestSearch_ForwardsLimit(t *testing.T) {
	var gotLimit string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{},
			"count":     0,
			"limit":     5,
			"has_more":  false,
		})
	})

	cmd := testutil.Command(newSearchCmd())
	cmd.SetArgs([]string{"--email", "buyer@example.com", "--limit", "5"})
	testutil.MustExecute(t, cmd)

	if gotLimit != "5" {
		t.Errorf("got limit=%q, want 5", gotLimit)
	}
}

func TestSearch_RejectsZeroLimit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not request when --limit is invalid")
	})

	cmd := testutil.Command(newSearchCmd())
	cmd.SetArgs([]string{"--email", "buyer@example.com", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected zero-limit error, got: %v", err)
	}
}

func TestSearch_EmptyResultMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{},
			"count":     0,
			"limit":     25,
			"has_more":  false,
		})
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No purchases found for buyer@example.com") {
		t.Errorf("expected empty-result message: %q", out)
	}
}

func TestSearch_HasMoreShowsTruncated(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{
				{"id": "1", "email": "buyer@example.com", "product_name": "Course", "purchase_state": "successful"},
			},
			"count":    25,
			"limit":    25,
			"has_more": true,
		})
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "(truncated)") {
		t.Errorf("expected truncated marker when has_more=true: %q", out)
	}
	if !strings.Contains(out, "Showing first 1 purchase(s) for buyer@example.com (truncated)") {
		t.Errorf("expected 'Showing first %%d ... (truncated)' framing when has_more=true, got: %q", out)
	}
	if strings.Contains(out, "of 25") {
		t.Errorf("must not display server-capped count as a total (server caps count at the limit, so 'of 25' would imply exactly 25 matches when there are more), got: %q", out)
	}
}

func TestSearch_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{
				{
					"id":                    "1",
					"email":                 "buyer@example.com",
					"product_name":          "Course",
					"formatted_total_price": "$12",
					"purchase_state":        "successful",
					"created_at":            "2026-04-24T12:00:00Z",
				},
				{
					"id":                    "2",
					"email":                 "buyer@example.com",
					"link_name":             "Bundle",
					"formatted_total_price": "$20",
					"purchase_state":        "refunded",
					"created_at":            "2026-04-23T12:00:00Z",
				},
			},
			"count":    2,
			"limit":    25,
			"has_more": false,
		})
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	wants := []string{
		"1\tbuyer@example.com\tCourse\t$12\tsuccessful\t2026-04-24T12:00:00Z",
		"2\tbuyer@example.com\tBundle\t$20\trefunded\t2026-04-23T12:00:00Z",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in plain output: %q", want, out)
		}
	}
}

func TestSearch_PlainOutputAmountAndStatusFallbacksMatchStyled(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{
				{
					"id":             "9",
					"email":          "buyer@example.com",
					"product_name":   "Course",
					"price_cents":    1200,
					"purchase_state": "successful",
					"refund_status":  "partially_refunded",
					"created_at":     "2026-04-24T12:00:00Z",
				},
			},
			"count":    1,
			"limit":    25,
			"has_more": false,
		})
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "9\tbuyer@example.com\tCourse\t1200 cents\tsuccessful, partially_refunded\t2026-04-24T12:00:00Z"
	if !strings.Contains(out, want) {
		t.Fatalf("plain row must derive amount from price_cents and combine purchase_state with refund_status (matching styled mode), got: %q", out)
	}
}

func TestSearch_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchases": []map[string]any{{"id": "1", "email": "buyer@example.com"}},
			"count":     1,
			"limit":     25,
			"has_more":  false,
		})
	})

	cmd := testutil.Command(newSearchCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Purchases []map[string]any `json:"purchases"`
		Count     int              `json:"count"`
		HasMore   bool             `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.Count != 1 || len(resp.Purchases) != 1 || resp.Purchases[0]["id"] != "1" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}
