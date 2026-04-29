package purchases

import (
	"encoding/json"
	"io"
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

func TestSearch_PostsEmailInBody(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body searchRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
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

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/search" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/search", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "buyer@example.com" {
		t.Errorf("got email %q, want buyer@example.com", body.Email)
	}
	for _, want := range []string{"1 purchase(s) for buyer@example.com", "Course", "Buyer: buyer@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: %q", want, out)
		}
	}
}

func TestSearch_OmitsLimitWhenNotSet(t *testing.T) {
	var body searchRequest
	var bodyKeys map[string]json.RawMessage

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.Unmarshal(raw, &bodyKeys); err != nil {
			t.Fatalf("decode body keys: %v", err)
		}
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

	if _, present := bodyKeys["limit"]; present {
		t.Errorf("limit must be omitted when not set, got body keys: %v", bodyKeys)
	}
}

func TestSearch_ForwardsLimit(t *testing.T) {
	var body searchRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
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

	if body.Limit != 5 {
		t.Errorf("got limit=%d, want 5", body.Limit)
	}
}

func TestSearch_RejectsZeroLimit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not POST when --limit is invalid")
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
