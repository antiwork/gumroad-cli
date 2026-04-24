package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestSuspensionUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotEmail, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"status":     "Suspended",
			"updated_at": "2026-04-24T12:00:00Z",
			"appeal_url": "https://gumroad.com/appeal",
		})
	})

	cmd := testutil.Command(newSuspensionCmd())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/suspension" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/suspension", gotMethod, gotPath)
	}
	if gotEmail != "user@example.com" {
		t.Fatalf("got email %q, want user@example.com", gotEmail)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{"user@example.com", "Status: Suspended", "Updated: 2026-04-24T12:00:00Z", "Appeal: https://gumroad.com/appeal"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestSuspensionRequiresEmail(t *testing.T) {
	cmd := newSuspensionCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuspensionJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"status": "Compliant"})
	})

	cmd := testutil.Command(newSuspensionCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.Status != "Compliant" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}
