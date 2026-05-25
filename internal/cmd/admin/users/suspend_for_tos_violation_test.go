package users

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestSuspendForTOSViolationRequiresUserID(t *testing.T) {
	cmd := newSuspendForTOSViolationCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuspendForTOSViolationRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not reach API without confirmation")
	})

	cmd := testutil.Command(newSuspendForTOSViolationCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestSuspendForTOSViolationConfirmationMessageDescribesPolicySuspension(t *testing.T) {
	got := fmt.Sprintf(suspendForTOSViolationConfirmationMessage, "2245593582708")
	for _, want := range []string{
		"Suspend user_id 2245593582708 for a policy violation?",
		"does not block them from buying as a customer",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("confirmation message missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "fraud") {
		t.Fatalf("confirmation message must not call this fraud: %q", got)
	}
}

func TestSuspendForTOSViolationSendsUserIDExpectedEmailAndSuspensionNote(t *testing.T) {
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
			"user_id": "2245593582708",
			"status":  "suspended_for_tos_violation",
			"message": "User suspended for a policy violation",
		})
	})

	cmd := testutil.Command(newSuspendForTOSViolationCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--note", "DMCA takedown notice confirmed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/suspend_for_tos_violation" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/suspend_for_tos_violation", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || body.SuspensionNote != "DMCA takedown notice confirmed" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User suspended for a policy violation", "User ID: 2245593582708", "Status: suspended_for_tos_violation"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestSuspendForTOSViolationDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the suspend_for_tos_violation endpoint")
	})

	cmd := testutil.Command(newSuspendForTOSViolationCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--note", "DMCA takedown notice confirmed"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/suspend_for_tos_violation") {
		t.Errorf("expected dry-run preview to mention POST and the suspend_for_tos_violation path, got: %q", out)
	}
	if !strings.Contains(out, "user_id: 2245593582708") || !strings.Contains(out, "expected_email: seller@example.com") || !strings.Contains(out, "suspension_note: DMCA takedown notice confirmed") {
		t.Errorf("expected dry-run preview to include user_id, expected_email, and suspension_note, got: %q", out)
	}
}

func TestSuspendForTOSViolationJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"user_id": "2245593582708",
			"status":  "already_suspended",
			"message": "User is already suspended for a policy violation",
		})
	})

	cmd := testutil.Command(newSuspendForTOSViolationCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp riskActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || resp.Status != "already_suspended" || resp.Message != "User is already suspended for a policy violation" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestSuspendForTOSViolationPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"user_id": "2245593582708",
			"status":  "suspended_for_tos_violation",
			"message": "User suspended for a policy violation",
		})
	})

	cmd := testutil.Command(newSuspendForTOSViolationCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tUser suspended for a policy violation\t2245593582708\tsuspended_for_tos_violation"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
