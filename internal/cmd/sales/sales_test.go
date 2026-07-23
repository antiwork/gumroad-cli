package sales

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func readCSVRecords(t *testing.T, value string) [][]string {
	t.Helper()

	records, err := csv.NewReader(strings.NewReader(value)).ReadAll()
	if err != nil {
		t.Fatalf("read CSV output: %v\n%s", err, value)
	}
	return records
}

func assertCSVRecords(t *testing.T, got, want [][]string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected CSV records:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestList_AllFilters(t *testing.T) {
	var gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--email", "a@b.com", "--order", "o1", "--before", "2024-12-31", "--after", "2024-01-01", "--page-key", "abc"})
	testutil.MustExecute(t, cmd)

	for _, param := range []string{"product_id=p1", "email=a%40b.com", "order_id=o1", "before=2024-12-31", "after=2024-01-01", "page_key=abc"} {
		if !strings.Contains(gotQuery, param) {
			t.Errorf("query missing param %q in %q", param, gotQuery)
		}
	}
}

func TestList_InvalidBeforeDate(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid date")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--before", "2024-13-01"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--before must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_Pagination(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales":         []map[string]any{{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"}},
			"next_page_key": "cursor123",
		})
	})
	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "cursor123") {
		t.Errorf("expected pagination hint with cursor, got: %q", out)
	}
}

func TestList_CSVOutput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":             "s1",
					"email":          "a@b.com",
					"product_name":   "Art, Pack",
					"total_cents":    1000,
					"currency":       "usd",
					"refunded":       true,
					"refunded_cents": 250,
					"created_at":     "2024-01-15T10:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{
		{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"},
		{"s1", "a@b.com", "Art, Pack", "1000", "usd", "true", "250", "2024-01-15T10:00:00Z", "", "", "", ""},
	}
	assertCSVRecords(t, records, want)
}

func TestList_CSVOutputIncludesBuyerPresentment(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":           "s1",
					"email":        "a@b.com",
					"product_name": "Art",
					"total_cents":  1500,
					"currency":     "usd",
					"created_at":   "2026-07-20T10:00:00Z",
					"buyer_presentment": map[string]any{
						"currency":       "cad",
						"total_cents":    2101,
						"refunded_cents": 700,
						"fx_rate":        "1.4005",
					},
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{
		{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"},
		{"s1", "a@b.com", "Art", "1500", "usd", "false", "0", "2026-07-20T10:00:00Z", "cad", "2101", "700", "1.4005"},
	}
	assertCSVRecords(t, records, want)
}

func TestList_CSVOutputLeavesNullBuyerAmountsEmpty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":           "s1",
					"email":        "a@b.com",
					"product_name": "Art",
					"total_cents":  1500,
					"currency":     "usd",
					"created_at":   "2026-07-20T10:00:00Z",
					"buyer_presentment": map[string]any{
						"currency":       "cad",
						"total_cents":    nil,
						"refunded_cents": nil,
						"fx_rate":        "1.4005",
					},
				},
				{
					"id":           "s2",
					"email":        "c@d.com",
					"product_name": "Pack",
					"total_cents":  2000,
					"currency":     "usd",
					"created_at":   "2026-07-21T10:00:00Z",
					"buyer_presentment": map[string]any{
						"currency": "eur",
						"fx_rate":  "0.9",
					},
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{
		{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"},
		{"s1", "a@b.com", "Art", "1500", "usd", "false", "0", "2026-07-20T10:00:00Z", "cad", "", "", "1.4005"},
		{"s2", "c@d.com", "Pack", "2000", "usd", "false", "0", "2026-07-21T10:00:00Z", "eur", "", "", "0.9"},
	}
	assertCSVRecords(t, records, want)
}

func TestList_TableShowsBuyerPresentmentTotal(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":                    "s1",
					"email":                 "a@b.com",
					"product_name":          "Art",
					"formatted_total_price": "$15",
					"created_at":            "2026-07-20",
					"buyer_presentment": map[string]any{
						"currency":    "cad",
						"total_cents": 2101,
						"fx_rate":     "1.4005",
					},
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "(buyer: 21.01 CAD)") {
		t.Fatalf("table output missing buyer presentment total, got %q", out)
	}
}

func TestList_TableShowsBuyerPresentmentTotalZeroDecimalCurrency(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":                    "s1",
					"email":                 "a@b.com",
					"product_name":          "Art",
					"formatted_total_price": "$15",
					"created_at":            "2026-07-20",
					"buyer_presentment": map[string]any{
						"currency":    "jpy",
						"total_cents": 2101,
						"fx_rate":     "140.05",
					},
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "(buyer: 2101 JPY)") {
		t.Fatalf("JPY has no minor unit so 2101 must render as 2101 JPY, got %q", out)
	}
}

func TestList_AllJSONRoundTripsFullBuyerPresentment(t *testing.T) {
	presentment := map[string]any{
		"currency":          "cad",
		"price_cents":       float64(1401),
		"tip_cents":         float64(280),
		"seller_tax_cents":  float64(175),
		"gumroad_tax_cents": float64(0),
		"shipping_cents":    float64(0),
		"total_cents":       float64(1856),
		"fx_rate":           "1.4005",
		"refunded_cents":    float64(0),
	}
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id": "s1", "email": "a@b.com", "product_name": "Art",
					"formatted_total_price": "$14.01", "created_at": "2026-07-20",
					"buyer_presentment": presentment,
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Sales []struct {
			BuyerPresentment map[string]any `json:"buyer_presentment"`
		} `json:"sales"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Sales) != 1 {
		t.Fatalf("got %d sales, want 1", len(resp.Sales))
	}
	if !reflect.DeepEqual(resp.Sales[0].BuyerPresentment, presentment) {
		t.Fatalf("--all --json must round-trip every buyer_presentment field:\ngot  %#v\nwant %#v", resp.Sales[0].BuyerPresentment, presentment)
	}
}

func TestList_CSVOutputWarnsWhenMorePagesExist(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"id": "s1", "email": "a@b.com", "product_name": "Art", "price": 1000, "currency_type": "usd", "created_at": "2024-01-15"},
			},
			"next_page_key": "cursor123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--after", "2024-01-01", "--csv"})
	stdout, stderr := testutil.CaptureOutput(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, stdout)
	want := [][]string{
		{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"},
		{"s1", "a@b.com", "Art", "1000", "usd", "false", "0", "2024-01-15", "", "", "", ""},
	}
	assertCSVRecords(t, records, want)
	if strings.Contains(stdout, "More results available") {
		t.Fatalf("CSV stdout must not include pagination warning, got %q", stdout)
	}

	wantHint := "More results available: gumroad sales list --product p1 --after 2024-01-01 --all --csv"
	if !strings.Contains(stderr, wantHint) {
		t.Fatalf("stderr missing pagination hint %q in %q", wantHint, stderr)
	}
}

func TestList_CSVOutputUsesCurrentSalesAPIFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":                    "s1",
					"email":                 "a@b.com",
					"product_name":          "Art",
					"price":                 1000,
					"currency_type":         "usd",
					"refunded":              false,
					"amount_refunded_cents": 0,
					"created_at":            "2024-01-15T10:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	if got := records[1][3]; got != "1000" {
		t.Fatalf("got total_cents %q, want 1000", got)
	}
	if got := records[1][4]; got != "usd" {
		t.Fatalf("got currency %q, want usd", got)
	}
	if got := records[1][6]; got != "0" {
		t.Fatalf("got refunded_cents %q, want 0", got)
	}
}

func TestList_CSVOutputSkipsNullPrimaryNumericFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"id":                    "s1",
					"email":                 "a@b.com",
					"product_name":          "Art",
					"total_cents":           nil,
					"price":                 1000,
					"currency":              "usd",
					"refunded":              true,
					"refunded_cents":        nil,
					"amount_refunded_cents": 250,
					"created_at":            "2024-01-15T10:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{
		{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"},
		{"s1", "a@b.com", "Art", "1000", "usd", "true", "250", "2024-01-15T10:00:00Z", "", "", "", ""},
	}
	assertCSVRecords(t, records, want)
}

func TestList_EmptyCSVOutputWritesHeader(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"}}
	assertCSVRecords(t, records, want)
}

func TestExport_QueuesSalesExportWithFilters(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"status":          "queued",
			"recipient_email": "seller@example.com",
		})
	})

	cmd := testutil.Command(newExportCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--from", "2026-01-01", "--to", "2026-05-21", "--product", "prod_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/sales/exports" {
		t.Fatalf("got %s %s, want POST /sales/exports", gotMethod, gotPath)
	}
	for key, want := range map[string]string{
		"from":       "2026-01-01",
		"to":         "2026-05-21",
		"product_id": "prod_123",
	} {
		if got := gotForm.Get(key); got != want {
			t.Fatalf("got %s=%q, want %q", key, got, want)
		}
	}
	if !strings.Contains(out, "CSV will be emailed to seller@example.com when ready.") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExport_QueuesSalesExportWithoutFilters(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"status":          "queued",
			"recipient_email": "seller@example.com",
		})
	})

	cmd := testutil.Command(newExportCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if len(gotForm) != 0 {
		t.Fatalf("expected no form filters, got %#v", gotForm)
	}
}

func TestExport_AcceptsListStyleDateAliases(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"status":          "queued",
			"recipient_email": "seller@example.com",
		})
	})

	cmd := testutil.Command(newExportCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--after", "2026-01-01", "--before", "2026-05-21"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := gotForm.Get("from"); got != "2026-01-01" {
		t.Fatalf("got from=%q, want 2026-01-01", got)
	}
	if got := gotForm.Get("to"); got != "2026-05-21" {
		t.Fatalf("got to=%q, want 2026-05-21", got)
	}
	if gotForm.Has("after") || gotForm.Has("before") {
		t.Fatalf("alias flags must not be sent to the server, got %#v", gotForm)
	}
}

func TestExport_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":          "queued",
			"recipient_email": "seller@example.com",
		})
	})

	cmd := testutil.Command(newExportCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--from", "2026-01-01"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success        bool   `json:"success"`
		Status         string `json:"status"`
		RecipientEmail string `json:"recipient_email"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "queued" || resp.RecipientEmail != "seller@example.com" {
		t.Fatalf("unexpected JSON response: %+v", resp)
	}
}

func TestExport_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":          "queued",
			"recipient_email": "seller@example.com",
		})
	})

	cmd := testutil.Command(newExportCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "queued\tseller@example.com" {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestExport_InvalidDate(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid date")
	})

	cmd := newExportCmd()
	cmd.SetArgs([]string{"--from", "2026-13-01"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--from must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExport_DuplicateDateAliasesRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with duplicate date flags")
	})

	cmd := newExportCmd()
	cmd.SetArgs([]string{"--from", "2026-01-01", "--after", "2026-01-02"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected duplicate date flag error")
	}
	if !strings.Contains(err.Error(), "--after cannot be combined with --from") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExport_DryRunSkipsNetwork(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run should not reach API")
	})

	cmd := testutil.Command(newExportCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"--from", "2026-01-01", "--to", "2026-05-21", "--product", "prod_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Dry run", "POST /sales/exports", "from: 2026-01-01", "to: 2026-05-21", "product_id: prod_123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSummary_RequestWithFilters(t *testing.T) {
	var gotMethod, gotPath string
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"gross_cents":    10000,
			"net_cents":      8500,
			"units":          12,
			"refunded_cents": 1500,
			"refunded_units": 3,
			"currency":       "usd",
			"from":           "2026-01-01",
			"to":             "2026-05-21",
		})
	})

	cmd := testutil.Command(newSummaryCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"--from", "2026-01-01", "--to", "2026-05-21", "--group-by", "month"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != "/sales/summary" {
		t.Fatalf("got %s %s, want GET /sales/summary", gotMethod, gotPath)
	}
	for key, want := range map[string]string{
		"from":     "2026-01-01",
		"to":       "2026-05-21",
		"group_by": "month",
	} {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("got %s=%q, want %q", key, got, want)
		}
	}
	for _, want := range []string{"2026-01-01..2026-05-21", "$100.00", "$85.00", "$15.00", "12", "3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSummary_DefaultsToServerDateRange(t *testing.T) {
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"gross_cents":    0,
			"net_cents":      0,
			"units":          0,
			"refunded_cents": 0,
			"refunded_units": 0,
			"currency":       "usd",
			"from":           "2026-04-22",
			"to":             "2026-05-21",
		})
	})

	cmd := testutil.Command(newSummaryCmd())
	cmd.SetArgs([]string{})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if len(gotQuery) != 0 {
		t.Fatalf("expected no query params, got %#v", gotQuery)
	}
}

func TestSummary_GroupByProductRendersBreakdown(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("group_by"); got != "product" {
			t.Fatalf("got group_by=%q, want product", got)
		}
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"gross_cents":    6000,
			"net_cents":      5500,
			"units":          3,
			"refunded_cents": 500,
			"refunded_units": 1,
			"currency":       "usd",
			"from":           "2026-01-01",
			"to":             "2026-01-31",
			"breakdown": []map[string]any{
				{"key": "prod_1", "label": "Course", "gross_cents": 5000, "net_cents": 4500, "units": 2, "refunded_cents": 500, "refunded_units": 1},
				{"key": "prod_2", "label": "Book", "gross_cents": 1000, "net_cents": 1000, "units": 1, "refunded_cents": 0, "refunded_units": 0},
			},
		})
	})

	cmd := testutil.Command(newSummaryCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"--group-by", "product"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"PRODUCT", "prod_1", "Course", "$50.00", "$45.00", "prod_2", "Book"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSummary_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"gross_cents":    10000,
			"net_cents":      8500,
			"units":          12,
			"refunded_cents": 1500,
			"refunded_units": 3,
			"currency":       "usd",
			"from":           "2026-01-01",
			"to":             "2026-05-21",
		})
	})

	cmd := testutil.Command(newSummaryCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--from", "2026-01-01"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool   `json:"success"`
		GrossCents int    `json:"gross_cents"`
		From       string `json:"from"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.GrossCents != 10000 || resp.From != "2026-01-01" {
		t.Fatalf("unexpected JSON response: %+v", resp)
	}
}

func TestSummary_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":        true,
			"gross_cents":    6000,
			"net_cents":      5500,
			"units":          3,
			"refunded_cents": 500,
			"refunded_units": 1,
			"currency":       "usd",
			"from":           "2026-01-01",
			"to":             "2026-01-31",
			"breakdown": []map[string]any{
				{"key": "2026-01", "label": "2026-01", "gross_cents": 6000, "net_cents": 5500, "units": 3, "refunded_cents": 500, "refunded_units": 1},
			},
		})
	})

	cmd := testutil.Command(newSummaryCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--group-by", "month"})
	out := strings.TrimSpace(testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) }))

	want := strings.Join([]string{
		"summary\t2026-01-01..2026-01-31\t\tusd\t6000\t5500\t3\t500\t1",
		"month\t2026-01\t2026-01\tusd\t6000\t5500\t3\t500\t1",
	}, "\n")
	if out != want {
		t.Fatalf("got plain output %q, want %q", out, want)
	}
}

func TestSummary_InvalidDate(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid date")
	})

	cmd := newSummaryCmd()
	cmd.SetArgs([]string{"--from", "2026-13-01"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--from must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSummary_InvalidGroupBy(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid group-by")
	})

	cmd := newSummaryCmd()
	cmd.SetArgs([]string{"--group-by", "email"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--group-by must be one of: product, day, week, month") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSummary_FromAfterToRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid range")
	})

	cmd := newSummaryCmd()
	cmd.SetArgs([]string{"--from", "2026-05-22", "--to", "2026-05-21"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--from must be on or before --to") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSummary_DateRangeTooWideRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with too-wide range")
	})

	cmd := newSummaryCmd()
	cmd.SetArgs([]string{"--from", "2025-01-01", "--to", "2026-05-21"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "date range cannot exceed 366 days") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSummary_FromOnlyDateRangeTooWideRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with too-wide range")
	})

	cmd := newSummaryCmd()
	cmd.SetArgs([]string{"--from", "2020-01-01"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "date range cannot exceed 366 days") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_AllFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
				},
				"next_page_key": "cursor123",
			})
		case "cursor123":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s2", "email": "b@c.com", "product_name": "Book", "formatted_total_price": "$12", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Sales []map[string]any `json:"sales"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Sales) != 2 {
		t.Fatalf("got %d sales, want 2", len(resp.Sales))
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_AllCSVOutputStreamsAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s1", "email": "a@b.com", "product_name": "Art", "price": 1000, "currency_type": "usd", "created_at": "2024-01-15"},
				},
				"next_page_key": "cursor123",
			})
		case "cursor123":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s2", "email": "b@c.com", "product_name": "Book", "price": 1200, "currency_type": "usd", "refunded": true, "amount_refunded_cents": 1200, "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--all", "--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{
		{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"},
		{"s1", "a@b.com", "Art", "1000", "usd", "false", "0", "2024-01-15", "", "", "", ""},
		{"s2", "b@c.com", "Book", "1200", "usd", "true", "1200", "2024-01-16", "", "", "", ""},
	}
	assertCSVRecords(t, records, want)
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_SinglePageDoesNotWalkPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if pageKey := r.URL.Query().Get("page_key"); pageKey != "" {
			t.Fatalf("unexpected page_key %q", pageKey)
		}
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
			},
			"next_page_key": "cursor123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Sales       []map[string]any `json:"sales"`
		NextPageKey string           `json:"next_page_key"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
	if len(resp.Sales) != 1 {
		t.Fatalf("got %d sales, want 1", len(resp.Sales))
	}
	if resp.NextPageKey != "cursor123" {
		t.Fatalf("got next_page_key=%q, want cursor123", resp.NextPageKey)
	}
}

func TestList_CSVRejectsOtherOutputModes(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
	}{
		{"json", testutil.Command(newListCmd(), testutil.JSONOutput())},
		{"jq", testutil.Command(newListCmd(), testutil.JQ(".sales"))},
		{"plain", testutil.Command(newListCmd(), testutil.PlainOutput())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not reach API with conflicting output flags")
			})

			tt.command.SetArgs([]string{"--csv"})
			err := tt.command.Execute()
			if err == nil {
				t.Fatal("expected conflicting output mode error")
			}
			if !strings.Contains(err.Error(), "--csv cannot be combined with --json, --jq, or --plain") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestList_AllJQFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
				},
				"next_page_key": "cursor123",
			})
		case "cursor123":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s2", "email": "b@c.com", "product_name": "Book", "formatted_total_price": "$12", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JQ(".sales | length"))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("got %q, want 2", out)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_AllPlainOutputStreamsAllPages(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
				},
				"next_page_key": "cursor123",
			})
		case "cursor123":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s2", "email": "b@c.com", "product_name": "Book", "formatted_total_price": "$12", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "s1") || !strings.Contains(out, "s2") {
		t.Fatalf("expected both pages in plain output, got %q", out)
	}
}

func TestList_AllOutputStreamsAllPages(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15", "refunded": true},
				},
				"next_page_key": "cursor123",
			})
		case "cursor123":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s2", "email": "b@c.com", "product_name": "Book", "formatted_total_price": "$12", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "s1") || !strings.Contains(out, "s2") || !strings.Contains(out, "refunded") {
		t.Fatalf("expected streamed table output, got %q", out)
	}
}

func TestList_AllOutputEmpty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No sales found") {
		t.Fatalf("expected empty message, got %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No sales found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestList_EmptyPageStillShowsPaginationHint(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales":         []any{},
			"next_page_key": "cursor123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--email", "buyer@example.com", "--order", "ord_123", "--before", "2024-12-31", "--after", "2024-01-01"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No sales found on this page.") {
		t.Fatalf("expected empty-page message, got %q", out)
	}
	want := "gumroad sales list --product p1 --email buyer@example.com --order ord_123 --before 2024-12-31 --after 2024-01-01 --page-key cursor123"
	if !strings.Contains(out, want) {
		t.Fatalf("expected pagination hint %q in %q", want, out)
	}
}

func TestView_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@b.com", "product_name": "Art",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
				"refunded": false, "shipped": false,
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if gotPath != "/sales/s1" {
		t.Errorf("got path %q, want /sales/s1", gotPath)
	}
	if !strings.Contains(out, "Art") {
		t.Errorf("output missing product name: %q", out)
	}
}

func TestView_RefundedStatus(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@b.com", "product_name": "Art",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
				"refunded": true,
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if !strings.Contains(out, "refunded") {
		t.Errorf("output should show refunded status: %q", out)
	}
}

func TestView_BuyerPresentment(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@b.com", "product_name": "Art",
				"formatted_total_price": "$15", "created_at": "2026-07-20",
				"buyer_presentment": map[string]any{
					"currency":       "cad",
					"total_cents":    2101,
					"refunded_cents": 0,
					"fx_rate":        "1.4005",
				},
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if !strings.Contains(out, "Buyer charged: 21.01 CAD") {
		t.Errorf("output should show buyer-charged amount: %q", out)
	}
	if !strings.Contains(out, "FX rate: 1.4005") {
		t.Errorf("output should show FX rate: %q", out)
	}
}

func TestView_NoBuyerPresentmentLinesForCanonicalSale(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@b.com", "product_name": "Art",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if strings.Contains(out, "Buyer charged") || strings.Contains(out, "FX rate") {
		t.Errorf("canonical sale must not show buyer presentment lines: %q", out)
	}
}

func TestView_RawFixture(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/view_raw.json"))
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if !strings.Contains(out, "Raw Art") || !strings.Contains(out, "raw@example.com") {
		t.Errorf("output missing raw fixture sale data: %q", out)
	}
}

func TestRefund_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"s1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
}

func TestRefund_DryRunSkipsConfirmationAndNetwork(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run should not reach API")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"s1", "--amount", "5.00"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Dry run", "PUT /sales/s1/refund", "amount_cents: 500"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestRefund_FullRefund(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if r.PostForm.Get("amount_cents") != "" {
			t.Error("full refund should not send amount_cents")
		}
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"s1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if gotMethod != "PUT" || gotPath != "/sales/s1/refund" {
		t.Errorf("got %s %s, want PUT /sales/s1/refund", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Sale s1 refunded.") {
		t.Errorf("expected full refund message, got %q", out)
	}
}

func TestRefund_Partial(t *testing.T) {
	var gotAmountCents string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotAmountCents = r.PostForm.Get("amount_cents")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"s1", "--amount", "5.00"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if gotAmountCents != "500" {
		t.Errorf("got amount_cents=%q, want 500", gotAmountCents)
	}
	if !strings.Contains(out, "Refunded 5.00 on sale s1.") {
		t.Errorf("expected partial refund message, got %q", out)
	}
}

func TestRefund_AmountInvalidInput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"s1", "--amount", "abc"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a valid amount") {
		t.Fatalf("expected validation error, got: %v", err)
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *cmdutil.UsageError, got %T", err)
	}
}

func TestRefund_AmountWholeNumberMessage(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"s1", "--amount", "5"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "Refunded 5.00 on sale s1.") {
		t.Errorf("expected normalized refund message, got %q", out)
	}
}

func TestRefund_AmountNoInputShowsNormalized(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"s1", "--amount", "5"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without confirmation")
	}
	// The error message should mention --yes (confirmation required), not raw "5"
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirmation error mentioning --yes, got: %v", err)
	}
}

func TestRefund_AmountZeroRejected(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"s1", "--amount", "0"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--amount must be greater than 0") {
		t.Fatalf("expected amount validation error, got: %v", err)
	}
}

func TestRefund_InvalidPartialAmount(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid amount")
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"s1", "--amount", "-1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--amount cannot be negative") {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Usage:", "refund <id>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func TestShip_InvalidTrackingURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid URL")
	})

	cmd := newShipCmd()
	cmd.SetArgs([]string{"s1", "--tracking-url", "ftp://example.com/track"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--tracking-url must use http or https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_PaginationHintPreservesFilters(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales":         []map[string]any{{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"}},
			"next_page_key": "cursor123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--email", "buyer@example.com", "--order", "ord_123", "--before", "2024-12-31", "--after", "2024-01-01"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "gumroad sales list --product p1 --email buyer@example.com --order ord_123 --before 2024-12-31 --after 2024-01-01 --page-key cursor123"
	if !strings.Contains(out, want) {
		t.Fatalf("expected replayable pagination hint %q in %q", want, out)
	}
}

func TestShip_TrackingURL(t *testing.T) {
	var gotMethod, gotPath, gotTrackingURL string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotTrackingURL = r.PostForm.Get("tracking_url")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newShipCmd()
	cmd.SetArgs([]string{"s1", "--tracking-url", "https://track.example.com/123"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if gotMethod != "PUT" || gotPath != "/sales/s1/mark_as_shipped" {
		t.Errorf("got %s %s, want PUT /sales/s1/mark_as_shipped", gotMethod, gotPath)
	}
	if gotTrackingURL != "https://track.example.com/123" {
		t.Errorf("got tracking_url=%q, want full URL", gotTrackingURL)
	}
}

func TestResendReceipt_UsesPost(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newResendReceiptCmd()
	testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if gotMethod != "POST" {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotPath != "/sales/s1/resend_receipt" {
		t.Errorf("got path %q, want /sales/s1/resend_receipt", gotPath)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{"id": "s1", "email": "a@b.com", "product_name": "Art"},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	sale := resp["sale"].(map[string]any)
	if sale["id"] != "s1" {
		t.Errorf("got id=%v, want s1", sale["id"])
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15", "refunded": false},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "s1") || !strings.Contains(out, "a@b.com") {
		t.Errorf("plain output missing data: %q", out)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@example.com", "product_name": "Art",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	var execErr error
	out := testutil.CaptureStdout(func() { execErr = cmd.RunE(cmd, []string{"s1"}) })
	if execErr != nil {
		t.Fatalf("RunE failed: %v", execErr)
	}
	cols := strings.Split(strings.TrimRight(out, "\n"), "\t")
	if len(cols) != 7 {
		t.Fatalf("expected 7 tab-separated columns, got %d: %q", len(cols), out)
	}
	if cols[0] != "s1" || cols[2] != "Art" || cols[3] != "$10" {
		t.Errorf("plain view data mismatch: %q", cols)
	}
	if cols[6] != "" {
		t.Errorf("order_id column should be empty when absent, got %q", cols[6])
	}
}

func TestView_PlainWithBuyerPresentmentAppendsColumn(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@example.com", "product_name": "Art",
				"formatted_total_price": "$15", "created_at": "2026-07-20",
				"buyer_presentment": map[string]any{
					"currency":    "cad",
					"total_cents": 2101,
					"fx_rate":     "1.4005",
				},
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	var execErr error
	out := testutil.CaptureStdout(func() { execErr = cmd.RunE(cmd, []string{"s1"}) })
	if execErr != nil {
		t.Fatalf("RunE failed: %v", execErr)
	}
	cols := strings.Split(strings.TrimRight(out, "\n"), "\t")
	if len(cols) != 8 {
		t.Fatalf("expected 8 tab-separated columns, got %d: %q", len(cols), out)
	}
	if cols[7] != "21.01 CAD" {
		t.Errorf("buyer-charged column mismatch, got %q", cols[7])
	}
}

func TestView_PlainWithOrderID(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@example.com", "product_name": "Art",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
				"order_id": 535572601,
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	var execErr error
	out := testutil.CaptureStdout(func() { execErr = cmd.RunE(cmd, []string{"s1"}) })
	if execErr != nil {
		t.Fatalf("RunE failed: %v", execErr)
	}
	cols := strings.Split(strings.TrimRight(out, "\n"), "\t")
	if len(cols) != 7 {
		t.Fatalf("expected 7 tab-separated columns, got %d: %q", len(cols), out)
	}
	if cols[0] != "s1" || cols[2] != "Art" || cols[3] != "$10" {
		t.Errorf("plain view data mismatch: %q", cols)
	}
	if cols[6] != "535572601" {
		t.Errorf("order_id column should be 535572601, got %q", cols[6])
	}
}

func TestView_ShippedStatus(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sale": map[string]any{
				"id": "s1", "email": "a@b.com", "product_name": "Physical",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
				"shipped": true,
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if !strings.Contains(out, "shipped") {
		t.Errorf("should show shipped status: %q", out)
	}
}

func TestView_OrderID(t *testing.T) {
	// Use raw JSON to test wire formats that Go's encoding/json normalizes away
	// (e.g. 535572601.0 as a float, explicit null).
	tests := []struct {
		name      string
		rawJSON   string // raw JSON response body
		wantShown string // non-empty means Order: line should contain this
	}{
		{"integer", `{"success":true,"sale":{"id":"s1","email":"a@example.com","product_name":"Art","formatted_total_price":"$10","created_at":"2024-01-15","order_id":535572601}}`, "535572601"},
		{"float", `{"success":true,"sale":{"id":"s1","email":"a@example.com","product_name":"Art","formatted_total_price":"$10","created_at":"2024-01-15","order_id":535572601.0}}`, "535572601"},
		{"zero", `{"success":true,"sale":{"id":"s1","email":"a@example.com","product_name":"Art","formatted_total_price":"$10","created_at":"2024-01-15","order_id":0}}`, ""},
		{"null", `{"success":true,"sale":{"id":"s1","email":"a@example.com","product_name":"Art","formatted_total_price":"$10","created_at":"2024-01-15","order_id":null}}`, ""},
		{"absent", `{"success":true,"sale":{"id":"s1","email":"a@example.com","product_name":"Art","formatted_total_price":"$10","created_at":"2024-01-15"}}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.rawJSON))
			})

			cmd := newViewCmd()
			var execErr error
			out := testutil.CaptureStdout(func() { execErr = cmd.RunE(cmd, []string{"s1"}) })
			if execErr != nil {
				t.Fatalf("RunE failed: %v", execErr)
			}
			if tt.wantShown != "" {
				if !strings.Contains(out, "Order: "+tt.wantShown) {
					t.Errorf("should show Order: %s, got: %q", tt.wantShown, out)
				}
			} else {
				if strings.Contains(out, "Order:") {
					t.Errorf("should not show Order line, got: %q", out)
				}
			}
		})
	}
}

func TestShip_WithoutTracking(t *testing.T) {
	var gotTrackingURL string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotTrackingURL = r.PostForm.Get("tracking_url")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newShipCmd()
	cmd.SetArgs([]string{"s1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if gotTrackingURL != "" {
		t.Errorf("ship without tracking should not send tracking_url, got %q", gotTrackingURL)
	}
}

func TestResendReceipt_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newResendReceiptCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if !strings.Contains(out, "Receipt resent") {
		t.Errorf("expected resent message, got: %q", out)
	}
}

func TestShip_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newShipCmd()
	cmd.SetArgs([]string{"s1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResendReceipt_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newResendReceiptCmd()
	err := cmd.RunE(cmd, []string{"s1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRefund_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := testutil.Command(newRefundCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"s1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{{"id": "s1", "email": "a@b.com"}},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestNewSalesCmd_Subcommands(t *testing.T) {
	cmd := NewSalesCmd()

	if cmd.Use != "sales" {
		t.Fatalf("got use=%q, want %q", cmd.Use, "sales")
	}
	for _, name := range []string{"list", "buyers", "summary", "export", "view", "refund", "ship", "resend-receipt"} {
		if child, _, err := cmd.Find([]string{name}); err != nil || child == nil || child.Name() != name {
			t.Fatalf("expected subcommand %q to be registered, got child=%v err=%v", name, child, err)
		}
	}
}

func TestList_All_InvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"sales":`)
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestList_All_SecondPageInvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"id": "s1", "email": "a@b.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
				},
				"next_page_key": "cursor123",
			})
		case "cursor123":
			testutil.RawJSON(t, w, `{"sales":`)
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected second-page parse error, got: %v", err)
	}
}

func TestView_InvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"sale":`)
	})

	cmd := newViewCmd()
	err := cmd.RunE(cmd, []string{"s1"})
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestList_AllAndPageKeyConflict(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with conflicting flags")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--all", "--page-key", "cursor123"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --all and --page-key together")
	}
	if !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("unexpected error: %v", err)
	}
}
