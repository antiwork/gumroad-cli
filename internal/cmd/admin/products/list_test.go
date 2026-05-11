package products

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func sampleProduct(id, name string) map[string]any {
	return map[string]any{
		"id":            id,
		"name":          name,
		"description":   "A short description",
		"price_cents":   20000,
		"currency_code": "usd",
		"permalink":     id,
		"long_url":      "https://gumroad.com/l/" + id,
		"preview_url":   "https://example.com/preview.jpg",
		"created_at":    "2026-05-01T12:00:00Z",
		"deleted_at":    nil,
		"alive":         true,
		"is_adult":      false,
		"seller": map[string]any{
			"id":    "u_123",
			"email": "seller@example.com",
		},
		"files": []map[string]any{
			{
				"id":           "f_1",
				"display_name": "Big Guide",
				"file_name":    "big-guide.pdf",
				"extension":    "PDF",
				"filegroup":    "document",
				"file_size":    1048576,
				"created_at":   "2026-05-01T12:00:00Z",
				"deleted_at":   nil,
			},
		},
	}
}

func sampleListPayload() map[string]any {
	return map[string]any{
		"products": []map[string]any{
			sampleProduct("abc123", "Art Pack"),
		},
		"pagination": map[string]any{
			"count": 47,
			"items": 10,
			"page":  1,
			"pages": 5,
			"prev":  nil,
			"next":  2,
			"last":  5,
		},
	}
}

func TestListRequiresEmailOrExternalID(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email/external-id error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email or --external-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRejectsNonPositivePage(t *testing.T) {
	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com", "--page", "0"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --page must be greater than 0 error")
	}
	if !strings.Contains(err.Error(), "--page must be greater than 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRejectsNonPositivePerPage(t *testing.T) {
	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com", "--per-page", "-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --per-page must be greater than 0 error")
	}
	if !strings.Contains(err.Error(), "--per-page must be greater than 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListUsesInternalAdminEndpointAndRendersHumanOutput(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotEmail, gotPage, gotPerPage string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		query := r.URL.Query()
		gotEmail = query.Get("email")
		gotPage = query.Get("page")
		gotPerPage = query.Get("per_page")
		testutil.JSON(t, w, sampleListPayload())
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--page", "1", "--per-page", "10"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/products" {
		t.Fatalf("got %s %s, want GET /internal/admin/products", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotEmail != "seller@example.com" || gotPage != "1" || gotPerPage != "10" {
		t.Fatalf("got query email=%q page=%q per_page=%q, want email=seller@example.com page=1 per_page=10", gotEmail, gotPage, gotPerPage)
	}
	for _, want := range []string{
		"Products for seller@example.com",
		"abc123",
		"Art Pack",
		"200.00 USD",
		"Page 1 of 5 (47 total)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestListOmitsPagingWhenFlagsUnset(t *testing.T) {
	var gotPage, gotPerPage string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		gotPage = query.Get("page")
		gotPerPage = query.Get("per_page")
		testutil.JSON(t, w, sampleListPayload())
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	testutil.MustExecute(t, cmd)

	if gotPage != "" || gotPerPage != "" {
		t.Fatalf("page/per_page must default to API when flags unset, got page=%q per_page=%q", gotPage, gotPerPage)
	}
}

func TestListMarksDeletedProducts(t *testing.T) {
	payload := sampleListPayload()
	deleted := sampleProduct("xyz789", "Old Pack")
	deleted["deleted_at"] = "2026-04-15T10:00:00Z"
	deleted["alive"] = false
	payload["products"] = append(payload["products"].([]map[string]any), deleted)

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Old Pack (deleted)") {
		t.Errorf("expected (deleted) suffix on the soft-deleted product name: %q", out)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected deleted status column: %q", out)
	}
}

func TestListEmptyResultStillShowsPaginationFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"products": []map[string]any{},
			"pagination": map[string]any{
				"count": 0, "items": 0, "page": 1, "pages": 0,
				"prev": nil, "next": nil, "last": 0,
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No products found for seller@example.com.") {
		t.Errorf("expected empty-result message: %q", out)
	}
	if !strings.Contains(out, "Page 1 of 1 (0 total)") {
		t.Errorf("expected pagination footer even on empty page: %q", out)
	}
}

func TestListMissingEmailFromAPISurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "email or external_id is required",
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected email/external_id required error")
	}
	if !strings.Contains(err.Error(), "email or external_id is required") {
		t.Errorf("missing underlying message: %v", err)
	}
}

func TestListAcceptsExternalIDInsteadOfEmail(t *testing.T) {
	var gotEmail, gotExternalID string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		gotEmail = query.Get("email")
		gotExternalID = query.Get("external_id")
		testutil.JSON(t, w, sampleListPayload())
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--external-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotEmail != "" {
		t.Errorf("query email must be empty when only --external-id is set, got %q", gotEmail)
	}
	if gotExternalID != "2245593582708" {
		t.Errorf("query external_id = %q, want 2245593582708", gotExternalID)
	}
	if !strings.Contains(out, "Products for external_id 2245593582708") {
		t.Errorf("expected header to mention external_id subject: %q", out)
	}
}

func TestListSendsBothWhenEmailAndExternalIDProvided(t *testing.T) {
	var gotEmail, gotExternalID string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		gotEmail = query.Get("email")
		gotExternalID = query.Get("external_id")
		testutil.JSON(t, w, sampleListPayload())
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "seller@example.com", "--external-id", "u_1"})
	testutil.MustExecute(t, cmd)

	if gotEmail != "seller@example.com" || gotExternalID != "u_1" {
		t.Fatalf("expected both email and external_id forwarded so the server can resolve, got email=%q external_id=%q", gotEmail, gotExternalID)
	}
}

func TestListUserNotFoundSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "User not found",
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--email", "missing@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected user-not-found error")
	}
	if !strings.Contains(err.Error(), "User not found") {
		t.Errorf("missing underlying message: %v", err)
	}
}

func TestListPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleListPayload())
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "abc123\tArt Pack\t200.00 USD\t1\talive\t2026-05-01T12:00:00Z"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\n got: %q\nwant: %q", out, want)
	}
}

func TestListJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleListPayload())
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool                     `json:"success"`
		Products   []map[string]interface{} `json:"products"`
		Pagination map[string]interface{}   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success {
		t.Fatalf("expected success=true: %s", out)
	}
	if len(resp.Products) != 1 || resp.Products[0]["id"] != "abc123" {
		t.Errorf("expected one product with id abc123, got %+v", resp.Products)
	}
	if resp.Pagination["pages"] != float64(5) {
		t.Errorf("expected pagination.pages=5, got %v", resp.Pagination["pages"])
	}
}
