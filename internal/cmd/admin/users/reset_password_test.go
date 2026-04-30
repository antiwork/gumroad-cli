package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestResetPassword_RequiresEmail(t *testing.T) {
	cmd := newResetPasswordCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResetPassword_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "user@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestResetPassword_PostsEmail(t *testing.T) {
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
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/reset_password" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/reset_password", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "user@example.com" {
		t.Fatalf("got email %q, want user@example.com", body.Email)
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
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/reset_password") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	if !strings.Contains(out, "email: user@example.com") {
		t.Errorf("expected email in dry-run preview, got: %q", out)
	}
}

func TestResetPassword_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"message": "Reset password instructions sent"})
	})

	cmd := testutil.Command(newResetPasswordCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "user@example.com"})
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
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tReset password instructions sent\tuser@example.com"
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
	cmd.SetArgs([]string{"--email", "missing@example.com"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "User not found") {
		t.Fatalf("expected not-found error to surface, got: %v", err)
	}
}
