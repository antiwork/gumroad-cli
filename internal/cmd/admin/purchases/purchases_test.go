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
