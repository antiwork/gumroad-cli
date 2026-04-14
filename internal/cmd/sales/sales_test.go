package sales

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

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
				"id": "s1", "email": "a@b.com", "product_name": "Art",
				"formatted_total_price": "$10", "created_at": "2024-01-15",
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"s1"}) })
	if !strings.Contains(out, "s1") || !strings.Contains(out, "Art") {
		t.Errorf("plain view missing data: %q", out)
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
	for _, name := range []string{"list", "view", "refund", "ship", "resend-receipt"} {
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
