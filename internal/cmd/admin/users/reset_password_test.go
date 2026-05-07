package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestResetPassword_RequiresUserID(t *testing.T) {
	cmd := newResetPasswordCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResetPassword_PostsUserID(t *testing.T) {
	var body resetPasswordRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if strings.Contains(string(raw), `"email"`) {
			t.Errorf("email field must be omitted when only --user-id is supplied, got %q", raw)
		}
		testutil.JSON(t, w, map[string]any{"message": "Reset password instructions sent"})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.UserID != "2245593582708" || body.ExpectedEmail != "" {
		t.Errorf("got user_id=%q expected_email=%q, want only user_id", body.UserID, body.ExpectedEmail)
	}
	if !strings.Contains(out, "Reset password instructions sent") {
		t.Errorf("expected success message: %q", out)
	}
}

func TestResetPassword_ForwardsExpectedEmail(t *testing.T) {
	var body resetPasswordRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{"message": "Reset password instructions sent"})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "user@example.com"})
	testutil.MustExecute(t, cmd)

	if body.UserID != "2245593582708" {
		t.Errorf("got user_id %q, want 2245593582708", body.UserID)
	}
	if body.ExpectedEmail != "user@example.com" {
		t.Errorf("got expected_email %q, want user@example.com", body.ExpectedEmail)
	}
}

func TestResetPassword_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestResetPassword_PostsUserIDToEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body resetPasswordRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message": "Reset password instructions sent",
		})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/reset_password" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/reset_password", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if body.UserID != "2245593582708" {
		t.Fatalf("got user_id %q, want 2245593582708", body.UserID)
	}
	if !strings.Contains(out, "Reset password instructions sent") {
		t.Errorf("expected success message: %q", out)
	}
}

func TestResetPassword_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/reset_password") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	for _, want := range []string{"user_id: 2245593582708", "expected_email: user@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in dry-run preview, got: %q", want, out)
		}
	}
}

func TestResetPassword_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"message": "Reset password instructions sent"})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Message != "Reset password instructions sent" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestResetPassword_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"message": "Reset password instructions sent"})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tReset password instructions sent\t2245593582708"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestResetPassword_UserNotFoundSurfaces(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "User not found",
		})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "missing"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "User not found") {
		t.Fatalf("expected not-found error to surface, got: %v", err)
	}
}
