package sales

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestBuyers_AggregatesAndDedupesAcrossPages(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{
						"email":        "a@example.com",
						"full_name":    "Alice",
						"created_at":   "2024-01-10",
						"utm_source":   "newsletter",
						"utm_medium":   "email",
						"utm_campaign": "winter",
						"utm_term":     "buyers",
						"utm_content":  "hero",
					},
				},
				"next_page_key": "cursor1",
			})
		case "cursor1":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{
						"email":        "a@example.com",
						"full_name":    "Alice Anderson",
						"created_at":   "2024-03-01",
						"utm_source":   "adwords",
						"utm_medium":   "cpc",
						"utm_campaign": "spring",
						"utm_term":     "template",
						"utm_content":  "footer",
					},
					{"email": "b@example.com", "full_name": "Bob", "created_at": "2024-02-01"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success {
		t.Fatalf("expected success, got %+v", resp)
	}
	want := []buyer{
		{
			Email:            "a@example.com",
			Name:             "Alice Anderson",
			PurchaseCount:    2,
			LastPurchaseDate: "2024-03-01",
			UTMSource:        "adwords",
			UTMMedium:        "cpc",
			UTMCampaign:      "spring",
			UTMTerm:          "template",
			UTMContent:       "footer",
		},
		{Email: "b@example.com", Name: "Bob", PurchaseCount: 1, LastPurchaseDate: "2024-02-01"},
	}
	if len(resp.Buyers) != len(want) {
		t.Fatalf("got %d buyers, want %d: %+v", len(resp.Buyers), len(want), resp.Buyers)
	}
	for i, b := range want {
		if resp.Buyers[i] != b {
			t.Fatalf("buyer %d = %+v, want %+v", i, resp.Buyers[i], b)
		}
	}

	var raw struct {
		Buyers []map[string]any `json:"buyers"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("not valid raw JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"} {
		value, ok := raw.Buyers[1][key]
		if !ok || value != "" {
			t.Fatalf("raw buyer JSON %s = %v, present %t; want present empty string", key, value, ok)
		}
	}
}

func TestBuyers_SortsByLastPurchaseDateDescendingThenEmail(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "old@example.com", "full_name": "Old", "created_at": "2024-01-01"},
				{"email": "newer@example.com", "full_name": "Newer", "created_at": "2024-05-01"},
				{"email": "alpha@example.com", "full_name": "Alpha", "created_at": "2024-05-01"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	gotEmails := make([]string, 0, len(resp.Buyers))
	for _, b := range resp.Buyers {
		gotEmails = append(gotEmails, b.Email)
	}
	wantEmails := []string{"alpha@example.com", "newer@example.com", "old@example.com"}
	if strings.Join(gotEmails, ",") != strings.Join(wantEmails, ",") {
		t.Fatalf("got order %v, want %v", gotEmails, wantEmails)
	}
}

func TestBuyers_UnionsAcrossProductsAndDedupes(t *testing.T) {
	var requestedProducts []string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		product := r.URL.Query().Get("product_id")
		requestedProducts = append(requestedProducts, product)
		switch product {
		case "p1":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"email": "a@example.com", "full_name": "Alice", "created_at": "2024-01-10"},
				},
			})
		case "p2":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"email": "a@example.com", "full_name": "Alice", "created_at": "2024-02-10"},
					{"email": "c@example.com", "full_name": "Carol", "created_at": "2024-02-15"},
				},
			})
		default:
			t.Fatalf("unexpected product_id %q", product)
		}
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--product", "p2"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	byEmail := map[string]buyer{}
	for _, b := range resp.Buyers {
		byEmail[b.Email] = b
	}
	if got := byEmail["a@example.com"]; got.PurchaseCount != 2 || got.LastPurchaseDate != "2024-02-10" {
		t.Fatalf("buyer a aggregated wrong: %+v", got)
	}
	if got := byEmail["c@example.com"]; got.PurchaseCount != 1 {
		t.Fatalf("buyer c aggregated wrong: %+v", got)
	}

	sort.Strings(requestedProducts)
	if strings.Join(requestedProducts, ",") != "p1,p2" {
		t.Fatalf("got requested products %v, want one p1 and one p2", requestedProducts)
	}
}

func TestBuyers_DuplicateProductFlagsCountedOnce(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("product_id"); got != "p1" {
			t.Fatalf("got product_id=%q, want p1", got)
		}
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "a@example.com", "full_name": "Alice", "created_at": "2024-01-10"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if requests != 1 {
		t.Fatalf("got %d requests, want 1 (duplicate --product must be deduped)", requests)
	}
	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 || resp.Buyers[0].PurchaseCount != 1 {
		t.Fatalf("expected single buyer counted once, got %+v", resp.Buyers)
	}
}

func TestBuyers_DedupesEmailCaseInsensitively(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "Buyer@Example.com", "full_name": "Buyer", "created_at": "2024-01-10"},
				{"email": "buyer@example.com", "full_name": "Buyer", "created_at": "2024-02-10"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 {
		t.Fatalf("expected 1 deduped buyer, got %+v", resp.Buyers)
	}
	if resp.Buyers[0].PurchaseCount != 2 {
		t.Fatalf("got count %d, want 2", resp.Buyers[0].PurchaseCount)
	}
	if resp.Buyers[0].Email != "Buyer@Example.com" {
		t.Fatalf("got email %q, want first-seen Buyer@Example.com", resp.Buyers[0].Email)
	}
}

func TestBuyers_SameDateNameTieKeepsFirstSeenAcrossProducts(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("product_id") {
		case "p1":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"email": "tie@example.com", "full_name": "First Seen", "created_at": "2024-06-01"},
				},
			})
		case "p2":
			testutil.JSON(t, w, map[string]any{
				"sales": []map[string]any{
					{"email": "tie@example.com", "full_name": "Second Seen", "created_at": "2024-06-01"},
				},
			})
		default:
			t.Fatalf("unexpected product_id %q", r.URL.Query().Get("product_id"))
		}
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--product", "p2"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 {
		t.Fatalf("expected 1 buyer, got %+v", resp.Buyers)
	}
	if resp.Buyers[0].Name != "First Seen" {
		t.Fatalf("got name %q, want First Seen (same-date tie must keep first-seen deterministically)", resp.Buyers[0].Name)
	}
	if resp.Buyers[0].PurchaseCount != 2 {
		t.Fatalf("got count %d, want 2", resp.Buyers[0].PurchaseCount)
	}
}

func TestBuyers_KeepsLatestNonEmptyName(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "d@example.com", "full_name": "Dave", "created_at": "2024-01-01"},
				{"email": "d@example.com", "full_name": "", "created_at": "2024-05-01"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 {
		t.Fatalf("expected 1 buyer, got %+v", resp.Buyers)
	}
	got := resp.Buyers[0]
	if got.Name != "Dave" {
		t.Fatalf("got name %q, want Dave (latest non-empty)", got.Name)
	}
	if got.LastPurchaseDate != "2024-05-01" {
		t.Fatalf("got last purchase %q, want 2024-05-01", got.LastPurchaseDate)
	}
}

func TestBuyers_UsesLatestAttributedPurchaseForUTMAtomically(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"email":        "d@example.com",
					"full_name":    "Dave",
					"created_at":   "2024-01-01",
					"utm_source":   "newsletter",
					"utm_medium":   "email",
					"utm_campaign": "winter",
					"utm_term":     "discount",
					"utm_content":  "hero",
				},
				{
					"email":        "d@example.com",
					"full_name":    "Dave",
					"created_at":   "2024-03-01",
					"utm_source":   "linkedin",
					"utm_campaign": "launch",
				},
				{"email": "d@example.com", "full_name": "Dave", "created_at": "2024-05-01"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 {
		t.Fatalf("expected 1 buyer, got %+v", resp.Buyers)
	}
	got := resp.Buyers[0]
	if got.LastPurchaseDate != "2024-05-01" {
		t.Fatalf("got last purchase %q, want latest sale 2024-05-01", got.LastPurchaseDate)
	}
	if got.UTMSource != "linkedin" || got.UTMMedium != "" || got.UTMCampaign != "launch" || got.UTMTerm != "" || got.UTMContent != "" {
		t.Fatalf("UTM fields = %+v, want latest attributed purchase fields without mixing older values", got)
	}
}

func TestBuyers_SkipsRowsWithoutEmail(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "", "full_name": "Anonymous", "created_at": "2024-01-10"},
				{"email": "a@example.com", "full_name": "Alice", "created_at": "2024-01-11"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 || resp.Buyers[0].Email != "a@example.com" {
		t.Fatalf("expected only the buyer with an email, got %+v", resp.Buyers)
	}
}

func TestBuyers_SendsDateFilters(t *testing.T) {
	var gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--before", "2024-12-31", "--after", "2024-01-01"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, param := range []string{"product_id=p1", "before=2024-12-31", "after=2024-01-01"} {
		if !strings.Contains(gotQuery, param) {
			t.Errorf("query missing param %q in %q", param, gotQuery)
		}
	}
}

func TestBuyers_NoProductAggregatesAllSales(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("product_id"); got != "" {
			t.Fatalf("expected no product_id filter, got %q", got)
		}
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "a@example.com", "full_name": "Alice", "created_at": "2024-01-10"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 {
		t.Fatalf("expected 1 buyer, got %+v", resp.Buyers)
	}
}

func TestBuyers_CSVOutput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"email":        "a@example.com",
					"full_name":    "Alice, Jr",
					"created_at":   "2024-03-01",
					"utm_source":   "newsletter",
					"utm_medium":   "email",
					"utm_campaign": "launch",
					"utm_term":     "buyers",
					"utm_content":  "header",
				},
				{"email": "a@example.com", "full_name": "Alice, Jr", "created_at": "2024-01-01"},
				{"email": "b@example.com", "full_name": "Bob", "created_at": "2024-02-01"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd())
	cmd.SetArgs([]string{"--product", "p1", "--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{
		{"email", "name", "purchase_count", "last_purchase_date", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"},
		{"a@example.com", "Alice, Jr", "2", "2024-03-01", "newsletter", "email", "launch", "buyers", "header"},
		{"b@example.com", "Bob", "1", "2024-02-01", "", "", "", "", ""},
	}
	assertCSVRecords(t, records, want)
}

func TestBuyers_EmptyCSVWritesHeader(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newBuyersCmd())
	cmd.SetArgs([]string{"--product", "p1", "--csv"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	records := readCSVRecords(t, out)
	want := [][]string{{"email", "name", "purchase_count", "last_purchase_date", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"}}
	assertCSVRecords(t, records, want)
}

func TestBuyers_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"email":        "a@example.com",
					"full_name":    "Alice",
					"created_at":   "2024-03-01",
					"utm_source":   "newsletter",
					"utm_medium":   "email",
					"utm_campaign": "launch",
					"utm_term":     "buyers",
					"utm_content":  "header",
				},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := strings.TrimSpace(testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) }))

	want := "a@example.com\tAlice\t1\t2024-03-01\tnewsletter\temail\tlaunch\tbuyers\theader"
	if out != want {
		t.Fatalf("got plain output %q, want %q", out, want)
	}
}

func TestBuyers_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{
					"email":        "a@example.com",
					"full_name":    "Alice",
					"created_at":   "2024-03-01",
					"utm_source":   "newsletter",
					"utm_medium":   "email",
					"utm_campaign": "launch",
					"utm_term":     "buyers",
					"utm_content":  "header",
				},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.Quiet(false), testutil.NoColor(true))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"EMAIL", "NAME", "PURCHASES", "LAST PURCHASE", "a@example.com", "Alice", "2024-03-01"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in table output %q", want, out)
		}
	}
	for _, notWant := range []string{"UTM", "newsletter", "launch"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("table output should stay four columns, but found %q in %q", notWant, out)
		}
	}
}

func TestBuyers_JQ(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "a@example.com", "full_name": "Alice", "created_at": "2024-03-01"},
				{"email": "b@example.com", "full_name": "Bob", "created_at": "2024-02-01"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JQ(".buyers | length"))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("got %q, want 2", out)
	}
}

func TestBuyers_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No buyers found.") {
		t.Fatalf("expected empty message, got %q", out)
	}
}

func TestBuyers_EmptyPlainIsSilent(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.PlainOutput(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty --plain output for pipe-friendliness, got %q", out)
	}
}

func TestBuyers_CapturesNameWhenCreatedAtMissing(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"sales": []map[string]any{
				{"email": "a@example.com", "full_name": "Anonymous Date"},
			},
		})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp buyersResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Buyers) != 1 || resp.Buyers[0].Name != "Anonymous Date" {
		t.Fatalf("expected name captured even without created_at, got %+v", resp.Buyers)
	}
}

func TestBuyers_EmptyJSONReturnsEmptyArray(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"sales": []any{}})
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, `"buyers": []`) {
		t.Fatalf("expected empty buyers array, got %q", out)
	}
}

func TestBuyers_CSVRejectsOtherOutputModes(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
	}{
		{"json", testutil.Command(newBuyersCmd(), testutil.JSONOutput())},
		{"jq", testutil.Command(newBuyersCmd(), testutil.JQ(".buyers"))},
		{"plain", testutil.Command(newBuyersCmd(), testutil.PlainOutput())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not reach API with conflicting output flags")
			})

			tt.command.SetArgs([]string{"--product", "p1", "--csv"})
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

func TestBuyers_InvalidAfterDate(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid date")
	})

	cmd := newBuyersCmd()
	cmd.SetArgs([]string{"--after", "2024-13-01"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--after must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuyers_InvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"sales":`)
	})

	cmd := testutil.Command(newBuyersCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}
