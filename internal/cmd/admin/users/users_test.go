package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestSuspensionUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotEmail, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		var payload suspensionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotEmail = payload.Email
		testutil.JSON(t, w, map[string]any{
			"status":     "Suspended",
			"updated_at": "2026-04-24T12:00:00Z",
			"appeal_url": "https://gumroad.com/appeal",
		})
	})

	cmd := testutil.Command(newSuspensionCmd())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/suspension" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/suspension", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("email should not be sent in query string, got %q", gotQuery)
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

func TestSuspensionPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":     "Flagged",
			"updated_at": "2026-04-24T12:00:00Z",
			"appeal_url": "https://gumroad.com/appeal",
		})
	})

	cmd := testutil.Command(newSuspensionCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "user@example.com\tFlagged\t2026-04-24T12:00:00Z\thttps://gumroad.com/appeal"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestSuspensionHumanOutputOmitsEmptyOptionalFields(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"status": "Compliant"})
	})

	cmd := testutil.Command(newSuspensionCmd())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "user@example.com\nStatus: Compliant" {
		t.Fatalf("unexpected human output: %q", out)
	}
}

func TestNewUsersCmdWiresSuspension(t *testing.T) {
	cmd := NewUsersCmd()
	if cmd.Use != "users" {
		t.Fatalf("Use = %q, want users", cmd.Use)
	}
	if got := cmd.Commands(); len(got) != 1 || got[0].Use != "suspension" {
		t.Fatalf("unexpected subcommands: %#v", got)
	}
}
