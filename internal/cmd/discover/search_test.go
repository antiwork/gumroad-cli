package discover

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func sampleResponse(products []map[string]any) map[string]any {
	if products == nil {
		products = []map[string]any{}
	}
	return map[string]any{
		"total":          len(products),
		"tags_data":      []any{},
		"filetypes_data": []any{},
		"products":       products,
	}
}

func sampleProduct(overrides map[string]any) map[string]any {
	p := map[string]any{
		"id":        "abc==",
		"permalink": "xmcug",
		"name":      "Create Animated Low Poly Characters",
		"seller": map[string]any{
			"id":          "1",
			"name":        "Crashsune",
			"avatar_url":  "https://files.example.com/avatar",
			"profile_url": "https://crashsune.example.com",
			"is_verified": false,
		},
		"ratings": map[string]any{
			"count":   249,
			"average": 5.0,
		},
		"thumbnail_url":        "https://files.example.com/thumb",
		"native_type":          "course",
		"quantity_remaining":   nil,
		"is_sales_limited":     false,
		"price_cents":          5999,
		"currency_code":        "usd",
		"is_pay_what_you_want": false,
		"url":                  "https://crashsune.example.com/l/LowPolyCourse",
		"duration_in_months":   nil,
		"recurrence":           nil,
		"description":          "Course description.",
	}
	for k, v := range overrides {
		p[k] = v
	}
	return p
}

func TestSearch_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != searchPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("query") != "design" {
			t.Errorf("query not forwarded; got %v", r.URL.Query())
		}
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"name": "Design Pack"}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.JSONOutput())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{"design"}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &resp); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jsonErr, out)
	}
	products, ok := resp["products"].([]any)
	if !ok || len(products) != 1 {
		t.Fatalf("unexpected products in JSON output: %v", resp["products"])
	}
}

func TestSearch_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"name": "Plain Row"}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"design"}) })
	if !strings.Contains(out, "Plain Row\t") {
		t.Errorf("plain output missing tab-separated name: %q", out)
	}
	if !strings.Contains(out, "Crashsune") {
		t.Errorf("plain output missing seller name: %q", out)
	}
}

func TestSearch_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"name": "Table Row", "price_cents": 1500}),
		}))
	})

	cmd := newSearchCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "Table Row") || !strings.Contains(out, "Crashsune") || !strings.Contains(out, "$15.00") {
		t.Errorf("table output missing expected columns: %q", out)
	}
}

func TestSearch_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse(nil))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"nonexistent"}) })
	if !strings.Contains(out, "No products found") {
		t.Errorf("expected empty state message, got: %q", out)
	}
}

func TestSearch_ForwardsFilters(t *testing.T) {
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, sampleResponse(nil))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	flagValues := map[string]string{
		"tag":          "font,illustration",
		"taxonomy":     "3d/games",
		"filetypes":    "pdf,epub",
		"min-price":    "5",
		"max-price":    "50",
		"rating":       "4",
		"min-reviews":  "10",
		"staff-picked": "true",
		"subscription": "true",
		"bundle":       "false",
		"call":         "true",
		"exclude-ids":  "abc,def",
		"sort":         "price_asc",
		"limit":        "12",
		"from":         "24",
	}
	for name, val := range flagValues {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}

	if err := cmd.RunE(cmd, []string{"design"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	checks := map[string]string{
		"query":             "design",
		"tags":              "font,illustration",
		"taxonomy":          "3d/games",
		"filetypes":         "pdf,epub",
		"min_price":         "5",
		"max_price":         "50",
		"rating":            "4",
		"min_reviews_count": "10",
		"staff_picked":      "true",
		"is_subscription":   "true",
		"is_bundle":         "false",
		"is_call":           "true",
		"exclude_ids":       "abc,def",
		"sort":              "price_asc",
		"size":              "12",
		"from":              "24",
	}
	for key, want := range checks {
		if got := gotQuery.Get(key); got != want {
			t.Errorf("query param %s: got %q, want %q", key, got, want)
		}
	}
}

func TestSearch_TriStateBooleansOmittedWhenUnset(t *testing.T) {
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, sampleResponse(nil))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.RunE(cmd, []string{"design"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	for _, key := range []string{"is_subscription", "is_bundle", "is_call", "rating", "staff_picked"} {
		if _, present := gotQuery[key]; present {
			t.Errorf("expected %s to be omitted when flag unset; got %v", key, gotQuery[key])
		}
	}
}

func TestSearch_RejectsRatingOutOfRange(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("rating validation must run before HTTP; got request: %s %s", r.Method, r.URL.Path)
	})

	for _, val := range []string{"0", "6", "-1"} {
		cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
		if err := cmd.Flags().Set("rating", val); err != nil {
			t.Fatalf("set rating=%s: %v", val, err)
		}
		err := cmd.RunE(cmd, []string{})
		if err == nil || !strings.Contains(err.Error(), "--rating must be between") {
			t.Errorf("rating=%s: expected validation error, got: %v", val, err)
		}
	}
}

func TestSearch_RejectsNegativeMinReviews(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("min-reviews validation must run before HTTP; got request: %s %s", r.Method, r.URL.Path)
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.Flags().Set("min-reviews", "-3"); err != nil {
		t.Fatalf("set min-reviews: %v", err)
	}
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "--min-reviews must not be negative") {
		t.Fatalf("expected min-reviews error, got: %v", err)
	}
}

func TestSearch_AcceptsAllSortValues(t *testing.T) {
	for _, sortVal := range []string{"best_sellers", "newest", "recently_updated", "staff_picked"} {
		var gotQuery url.Values
		testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.Query()
			testutil.JSON(t, w, sampleResponse(nil))
		})

		cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
		if err := cmd.Flags().Set("sort", sortVal); err != nil {
			t.Fatalf("set sort=%s: %v", sortVal, err)
		}
		if err := cmd.RunE(cmd, []string{}); err != nil {
			t.Fatalf("sort=%s RunE: %v", sortVal, err)
		}
		if got := gotQuery.Get("sort"); got != sortVal {
			t.Errorf("sort=%s: forwarded %q, want %q", sortVal, got, sortVal)
		}
	}
}

func TestSearch_DefaultSortOmitsParam(t *testing.T) {
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, sampleResponse(nil))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.RunE(cmd, []string{"design"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if got := gotQuery.Get("sort"); got != "" {
		t.Errorf("default --sort must omit the sort param; got sort=%q", got)
	}
}

func TestSearch_NoQueryOmitsParam(t *testing.T) {
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, sampleResponse(nil))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if _, present := gotQuery["query"]; present {
		t.Errorf("expected no query param when no positional argument; got %v", gotQuery)
	}
}

func TestSearch_RejectsInvalidSort(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("invalid sort must fail before HTTP; got request: %s %s", r.Method, r.URL.Path)
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.Flags().Set("sort", "totally-invalid"); err != nil {
		t.Fatalf("set sort: %v", err)
	}
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "invalid --sort") {
		t.Fatalf("expected invalid-sort error, got: %v", err)
	}
}

func TestSearch_RejectsInvalidLimit(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("limit validation must run before HTTP; got request: %s %s", r.Method, r.URL.Path)
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.Flags().Set("limit", "9999"); err != nil {
		t.Fatalf("set limit: %v", err)
	}
	if err := cmd.RunE(cmd, []string{}); err == nil || !strings.Contains(err.Error(), "--limit must not exceed") {
		t.Fatalf("expected limit cap error, got: %v", err)
	}
}

func TestSearch_RejectsMinAboveMax(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request")
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.Flags().Set("min-price", "50"); err != nil {
		t.Fatalf("set min-price: %v", err)
	}
	if err := cmd.Flags().Set("max-price", "20"); err != nil {
		t.Fatalf("set max-price: %v", err)
	}
	if err := cmd.RunE(cmd, []string{}); err == nil || !strings.Contains(err.Error(), "--min-price cannot exceed --max-price") {
		t.Fatalf("expected min-above-max error, got: %v", err)
	}
}

func TestSearch_PaywhatyouwantRendersInPlain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"is_pay_what_you_want": true, "price_cents": 0, "name": "PWYW Item"}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "PWYW") {
		t.Errorf("plain output missing PWYW marker: %q", out)
	}
}

func TestSearch_FreeProduct(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"price_cents": 0, "name": "Freebie"}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "Free") {
		t.Errorf("plain output missing Free price: %q", out)
	}
}

func TestSearch_RecurringSubscriptionShowsCadence(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"price_cents": 2000, "recurrence": "monthly", "name": "Sub"}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "$20.00 / monthly") {
		t.Errorf("plain output missing recurrence: %q", out)
	}
}

func TestSearch_NoAuthorizationHeaderEverSent(t *testing.T) {
	var sawAuth atomic.Bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			sawAuth.Store(true)
		}
		testutil.JSON(t, w, sampleResponse(nil))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	if err := cmd.RunE(cmd, []string{"design"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if sawAuth.Load() {
		t.Fatalf("Authorization header was sent to discover endpoint; expected anonymous request")
	}
}

func TestSearch_TruncatesLongName(t *testing.T) {
	long := strings.Repeat("A", 80)
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"name": long}),
		}))
	})

	cmd := newSearchCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if strings.Contains(out, long) {
		t.Errorf("table output must truncate long names; got full name in:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected ellipsis in truncated output: %q", out)
	}
}

func TestSearch_NonUSDCurrencyFormats(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"price_cents": 1234, "currency_code": "eur", "name": "Euro Item"}),
			sampleProduct(map[string]any{"price_cents": 1500, "currency_code": "eur", "recurrence": "yearly", "name": "Euro Sub"}),
			sampleProduct(map[string]any{"price_cents": 500, "currency_code": "", "name": "Missing Currency"}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "12.34 EUR") {
		t.Errorf("missing non-USD price: %q", out)
	}
	if !strings.Contains(out, "15.00 EUR / yearly") {
		t.Errorf("missing non-USD recurring price: %q", out)
	}
	if !strings.Contains(out, "$5.00") {
		t.Errorf("missing currency-fallback price: %q", out)
	}
}

func TestSearch_RatingZeroCountShowsDash(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleResponse([]map[string]any{
			sampleProduct(map[string]any{"name": "Unrated", "ratings": map[string]any{"count": 0, "average": 0.0}}),
		}))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "Unrated\t") || !strings.Contains(out, "\t-\t") {
		t.Errorf("expected dash for zero-rating row: %q", out)
	}
}

func TestNewDiscoverCmd_RegistersSearch(t *testing.T) {
	cmd := NewDiscoverCmd()
	if cmd.Use != "discover" {
		t.Errorf("got Use=%q, want discover", cmd.Use)
	}
	subs := cmd.Commands()
	found := false
	for _, c := range subs {
		if strings.HasPrefix(c.Use, "search") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected discover to register search subcommand; got %v", subs)
	}
}

func TestSearch_HTTP5xxSurfaces(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"message":"oh no"}`))
	})

	cmd := testutil.Command(newSearchCmd(), testutil.Quiet(true))
	err := cmd.RunE(cmd, []string{"design"})
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
}

func TestPublicBaseURLDefaultsToGumroadCom(t *testing.T) {
	t.Setenv("GUMROAD_API_BASE_URL", "")
	if got := publicBaseURL(); got != defaultPublicBaseURL {
		t.Errorf("publicBaseURL() = %q, want %q — search must hit the public host, not the v2 API", got, defaultPublicBaseURL)
	}
}

func TestPublicBaseURLRespectsEnvOverride(t *testing.T) {
	t.Setenv("GUMROAD_API_BASE_URL", "https://staging.example.com")
	if got := publicBaseURL(); got != "https://staging.example.com" {
		t.Errorf("publicBaseURL() = %q, want env override to win", got)
	}
}
