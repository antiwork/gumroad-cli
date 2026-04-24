package licenses

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestLookupUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotKey, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotKey = r.URL.Query().Get("license_key")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"license": map[string]any{
				"email": "buyer@example.com", "product_name": "Course", "purchase_id": "123",
				"uses": 2, "enabled": true,
			},
		})
	})

	cmd := testutil.Command(newLookupCmd())
	cmd.SetArgs([]string{"--key", "ABC-123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/licenses/lookup" {
		t.Fatalf("got %s %s, want GET /internal/admin/licenses/lookup", gotMethod, gotPath)
	}
	if gotKey != "ABC-123" {
		t.Fatalf("got license_key %q, want ABC-123", gotKey)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{"Course", "Purchase ID: 123", "Buyer: buyer@example.com", "Uses: 2", "Status: enabled"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "ABC-123") {
		t.Fatalf("human output should not echo license key: %q", out)
	}
}

func TestLookupReadsLicenseKeyFromStdin(t *testing.T) {
	var gotKey string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("license_key")
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"email": "buyer@example.com", "product_id": "prod_123"},
			"uses":     1,
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.Stdin(strings.NewReader("PIPE-KEY\n")))
	cmd.SetArgs([]string{})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotKey != "PIPE-KEY" {
		t.Fatalf("got license_key %q, want PIPE-KEY", gotKey)
	}
}

func TestLookupJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"license": map[string]any{"purchase_id": "123", "uses": 2},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--key", "ABC-123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		License map[string]any `json:"license"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.License["purchase_id"] != "123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestLookupRejectsEmptyKeyFlag(t *testing.T) {
	cmd := newLookupCmd()
	cmd.SetArgs([]string{"--key", ""})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected empty key error")
	}
	if !strings.Contains(err.Error(), "--key cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
