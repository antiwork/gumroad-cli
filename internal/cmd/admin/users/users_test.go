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
		gotAuth = r.Header.Get("Authorization")
		gotEmail = r.URL.Query().Get("email")
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

func TestSuspensionRequiresEmailOrUserID(t *testing.T) {
	cmd := newSuspensionCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "supply --email or --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuspensionResolvesByUserID(t *testing.T) {
	var gotEmail, gotUserID string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, map[string]any{
			"user_id":    "2245593582708",
			"status":     "Suspended",
			"updated_at": "2026-04-24T12:00:00Z",
		})
	})

	cmd := testutil.Command(newSuspensionCmd())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotEmail != "" || gotUserID != "2245593582708" {
		t.Errorf("got email=%q user_id=%q, want only user_id", gotEmail, gotUserID)
	}
	if !strings.Contains(out, "2245593582708") {
		t.Errorf("expected user_id in headline output: %q", out)
	}
	if strings.Contains(out, "User ID: 2245593582708") {
		t.Errorf("must not repeat user_id immediately after the headline: %q", out)
	}
}

func TestSuspensionForwardsBothEmailAndUserID(t *testing.T) {
	var gotEmail, gotUserID string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, map[string]any{"status": "Compliant"})
	})

	cmd := testutil.Command(newSuspensionCmd())
	cmd.SetArgs([]string{"--email", "user@example.com", "--user-id", "2245593582708"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotEmail != "user@example.com" || gotUserID != "2245593582708" {
		t.Errorf("got email=%q user_id=%q, want both forwarded", gotEmail, gotUserID)
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

func TestSuspensionHumanOutputOmitsEmptyStatus(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newSuspensionCmd())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "user@example.com" {
		t.Fatalf("unexpected human output: %q", out)
	}
}

func TestNewUsersCmdWiresSubcommands(t *testing.T) {
	cmd := NewUsersCmd()
	if cmd.Use != "users" {
		t.Fatalf("Use = %q, want users", cmd.Use)
	}

	got := cmd.Commands()
	want := []string{
		"info",
		"affiliates",
		"comments",
		"compliance",
		"purchases",
		"related",
		"suspension",
		"mark-compliant",
		"watch",
		"update-watch",
		"unwatch",
		"suspend",
		"reset-password",
		"update-email",
		"two-factor",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d subcommands, got %d: %#v", len(want), len(got), got)
	}

	names := map[string]bool{}
	for _, sub := range got {
		names[sub.Use] = true
	}
	for _, name := range want {
		if !names[name] {
			t.Errorf("missing subcommand %q in %v", name, names)
		}
	}
}
