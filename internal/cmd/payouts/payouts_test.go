package payouts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/payouts" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	payouts := resp["payouts"].([]any)
	if len(payouts) != 2 {
		t.Errorf("got %d payouts, want 2", len(payouts))
	}
}

func TestList_AllJQFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JQ(".payouts | length"))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("got %q, want 2", out)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_NoUpcoming(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Contains(out, "pay2") {
		t.Errorf("--no-upcoming should filter out upcoming payouts: %q", out)
	}
	if !strings.Contains(out, "pay1") {
		t.Errorf("should still show non-upcoming payout: %q", out)
	}
}

func TestList_Filters(t *testing.T) {
	var gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{"payouts": []any{}})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--before", "2024-06-01", "--after", "2024-01-01", "--page-key", "xyz"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, param := range []string{"before=2024-06-01", "after=2024-01-01", "page_key=xyz"} {
		if !strings.Contains(gotQuery, param) {
			t.Errorf("query missing param %q in %q", param, gotQuery)
		}
	}
}

func TestList_InvalidAfterDate(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid date")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--after", "2024-02-30"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--after must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestView_Flags(t *testing.T) {
	var gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{
				"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100",
				"transactions": []map[string]any{{"id": "txn1"}, {"id": "txn2"}},
			},
		})
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"pay1", "--no-sales", "--include-transactions"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(gotQuery, "include_sales=false") {
		t.Errorf("--no-sales should send include_sales=false, got: %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "include_transactions=true") {
		t.Errorf("--include-transactions should send include_transactions=true, got: %q", gotQuery)
	}
	for _, want := range []string{"Sales: omitted (--no-sales)", "Transactions: 2 included"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestUpcoming_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{"display_payout_period": "Mar 2024", "formatted_amount": "$200"},
		})
	})

	cmd := newUpcomingCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/payouts/upcoming" {
		t.Errorf("got path %q, want /payouts/upcoming", gotPath)
	}
	if !strings.Contains(out, "$200") {
		t.Errorf("output missing amount: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "pay1") {
		t.Errorf("plain output missing data: %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"payouts": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No payouts found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestList_AmountCentsFloat(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"payouts": [{
				"id": "pay1",
				"amount_cents": 10000.0,
				"display_payout_period": "Jan 2024",
				"formatted_amount": "$100",
				"is_upcoming": false
			}]
		}`)
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "pay1") || !strings.Contains(out, "$100") {
		t.Fatalf("output missing payout data: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100"},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"pay1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100"},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"pay1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "pay1") {
		t.Errorf("plain view missing data: %q", out)
	}
}

func TestView_PlainShowsRequestedDetails(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{
				"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100",
				"transactions": []map[string]any{{"id": "txn1"}, {"id": "txn2"}},
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"pay1", "--no-sales", "--include-transactions"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "pay1\tJan 2024\t$100\tomitted\t2" {
		t.Fatalf("unexpected plain detail output: %q", out)
	}
}

func TestUpcoming_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{"display_payout_period": "Mar 2024", "formatted_amount": "$200"},
		})
	})

	cmd := testutil.Command(newUpcomingCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestUpcoming_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{"display_payout_period": "Mar 2024", "formatted_amount": "$200"},
		})
	})

	cmd := testutil.Command(newUpcomingCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "$200") {
		t.Errorf("plain output missing amount: %q", out)
	}
}

func TestUpcoming_ShowsRequestedDetails(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{
				"display_payout_period": "Mar 2024",
				"formatted_amount":      "$200",
				"transactions":          []map[string]any{{"id": "txn1"}},
			},
		})
	})

	cmd := newUpcomingCmd()
	cmd.SetArgs([]string{"--no-sales", "--include-transactions"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	for _, want := range []string{"Sales: omitted (--no-sales)", "Transactions: 1 included"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestView_UpcomingStatus(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payout": map[string]any{
				"id": "pay1", "display_payout_period": "Mar 2024",
				"formatted_amount": "$200", "is_upcoming": true,
			},
		})
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"pay1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "upcoming") {
		t.Errorf("should show upcoming status: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_UpcomingTag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Mar 2024", "formatted_amount": "$200", "is_upcoming": true},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "upcoming") {
		t.Errorf("should show upcoming tag: %q", out)
	}
}

func TestList_Pagination(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
			},
			"next_page_key": "cursor456",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "cursor456") {
		t.Errorf("expected pagination hint, got: %q", out)
	}
}

func TestList_AllFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				},
				"next_page_key": "cursor456",
			})
		case "cursor456":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": false},
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
		Payouts []map[string]any `json:"payouts"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Payouts) != 2 {
		t.Fatalf("got %d payouts, want 2", len(resp.Payouts))
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
				"payouts": []map[string]any{
					{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				},
				"next_page_key": "cursor456",
			})
		case "cursor456":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "pay1") || !strings.Contains(out, "pay2") {
		t.Fatalf("expected both pages in plain output, got %q", out)
	}
}

func TestList_AllOutputStreamsAllPages(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				},
				"next_page_key": "cursor456",
			})
		case "cursor456":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "pay1") || !strings.Contains(out, "pay2") || !strings.Contains(out, "upcoming") {
		t.Fatalf("expected streamed table output, got %q", out)
	}
}

func TestList_AllOutputEmpty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"payouts": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No payouts found") {
		t.Fatalf("expected empty message, got %q", out)
	}
}

func TestList_All_SecondPageInvalidJSON(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"payouts": []map[string]any{
					{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				},
				"next_page_key": "cursor456",
			})
		case "cursor456":
			testutil.RawJSON(t, w, `{"payouts":`)
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
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_JSONRespectsNoUpcoming(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
				{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Payouts []map[string]any `json:"payouts"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Payouts) != 1 || resp.Payouts[0]["id"] != "pay1" {
		t.Fatalf("unexpected payouts payload: %s", out)
	}
}

func TestList_NoUpcomingDoesNotSkipAhead(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if pageKey := r.URL.Query().Get("page_key"); pageKey != "" {
			t.Fatalf("unexpected page_key %q", pageKey)
		}
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay-upcoming", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": true},
			},
			"next_page_key": "cursor456",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Payouts     []map[string]any `json:"payouts"`
		NextPageKey string           `json:"next_page_key"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
	if len(resp.Payouts) != 0 {
		t.Fatalf("unexpected payouts payload: %s", out)
	}
	if resp.NextPageKey != "cursor456" {
		t.Fatalf("got next_page_key=%q, want cursor456", resp.NextPageKey)
	}
}

func TestList_NoUpcomingEmptyPageStillShowsPaginationHint(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay-upcoming", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": true},
			},
			"next_page_key": "cursor456",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--before", "2024-06-01", "--after", "2024-01-01", "--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No payouts found on this page.") {
		t.Fatalf("expected empty-page message, got %q", out)
	}
	want := "gumroad payouts list --before 2024-06-01 --after 2024-01-01 --no-upcoming --page-key cursor456"
	if !strings.Contains(out, want) {
		t.Fatalf("expected pagination hint %q in %q", want, out)
	}
}

func TestList_NoUpcoming_JSON_PreservesUnknownFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"payouts": [
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false, "extra_field": "preserved"},
				{"id": "pay2", "display_payout_period": "Feb 2024", "formatted_amount": "$50", "is_upcoming": true, "extra_field": "filtered"}
			],
			"unknown_top_level": "should survive"
		}`)
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}

	// Unknown top-level field must survive
	if resp["unknown_top_level"] != "should survive" {
		t.Fatalf("unknown top-level field lost: %s", out)
	}

	// Only non-upcoming payout should remain
	payouts := resp["payouts"].([]any)
	if len(payouts) != 1 {
		t.Fatalf("got %d payouts, want 1", len(payouts))
	}

	payout := payouts[0].(map[string]any)
	if payout["id"] != "pay1" {
		t.Fatalf("wrong payout survived filter: %v", payout["id"])
	}

	// Unknown per-item field must survive
	if payout["extra_field"] != "preserved" {
		t.Fatalf("unknown per-item field lost: %s", out)
	}
}

func TestList_NoUpcoming_JSON_PreservesSuccessField(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"payouts": [
				{"id": "pay1", "is_upcoming": false}
			]
		}`)
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp["success"] != true {
		t.Fatalf("success field not preserved: got %v", resp["success"])
	}
}

func TestList_PaginationHintPreservesFilters(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"payouts": []map[string]any{
				{"id": "pay1", "display_payout_period": "Jan 2024", "formatted_amount": "$100", "is_upcoming": false},
			},
			"next_page_key": "cursor456",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--before", "2024-06-01", "--after", "2024-01-01", "--no-upcoming"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "gumroad payouts list --before 2024-06-01 --after 2024-01-01 --no-upcoming --page-key cursor456"
	if !strings.Contains(out, want) {
		t.Fatalf("expected replayable pagination hint %q in %q", want, out)
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
