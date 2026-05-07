package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestSuspendRequiresEmailOrExternalID(t *testing.T) {
	cmd := newSuspendCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "supply --email or --external-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuspendSendsExternalID(t *testing.T) {
	var body suspendRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if strings.Contains(string(raw), `"email"`) {
			t.Errorf("email field must be omitted when only --external-id is supplied, got %q", raw)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "suspended_for_fraud",
			"message": "User suspended for fraud",
		})
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--external-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.ExternalID != "2245593582708" || body.Email != "" {
		t.Errorf("got email=%q external_id=%q, want only external_id", body.Email, body.ExternalID)
	}
	if !strings.Contains(out, "External ID: 2245593582708") {
		t.Errorf("expected External ID label when only --external-id is supplied: %q", out)
	}
}

func TestSuspendForwardsBothEmailAndExternalID(t *testing.T) {
	var body suspendRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "suspended_for_fraud",
			"message": "User suspended for fraud",
		})
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--email", "seller@example.com",
		"--external-id", "2245593582708",
	})
	testutil.MustExecute(t, cmd)

	if body.Email != "seller@example.com" || body.ExternalID != "2245593582708" {
		t.Errorf("got email=%q external_id=%q, want both forwarded", body.Email, body.ExternalID)
	}
}

func TestSuspendRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestSuspendSendsEmailAndSuspensionNote(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var body suspendRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "suspended_for_fraud",
			"message": "User suspended for fraud",
		})
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--note", "Chargeback risk confirmed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/suspend_for_fraud" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/suspend_for_fraud", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("email/note must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.Email != "seller@example.com" || body.SuspensionNote != "Chargeback risk confirmed" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User suspended for fraud", "Email: seller@example.com", "Status: suspended_for_fraud"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestSuspendDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the suspend_for_fraud endpoint")
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--note", "Chargeback risk confirmed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/suspend_for_fraud") {
		t.Errorf("expected dry-run preview to mention POST and the suspend_for_fraud path, got: %q", out)
	}
	if !strings.Contains(out, "email: seller@example.com") || !strings.Contains(out, "suspension_note: Chargeback risk confirmed") {
		t.Errorf("expected dry-run preview to include email and suspension_note, got: %q", out)
	}
}

func TestSuspendJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "already_suspended",
			"message": "User is already suspended for fraud",
		})
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp riskActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "already_suspended" || resp.Message != "User is already suspended for fraud" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestSuspendPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "suspended_for_fraud",
			"message": "User suspended for fraud",
		})
	})

	cmd := testutil.Command(newSuspendCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tUser suspended for fraud\tseller@example.com\tsuspended_for_fraud"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
