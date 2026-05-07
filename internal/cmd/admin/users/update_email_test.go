package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUpdateEmail_RequiresIdentifierAndNewEmail(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing-identifier", []string{"--new-email", "new@example.com"}, "missing required flag: --user-id"},
		{"missing-new-with-user-id", []string{"--user-id", "2245593582708"}, "missing required flag: --new-email"},
		{"missing-new-with-external-id-alias", []string{"--external-id", "2245593582708"}, "missing required flag: --new-email"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newUpdateEmailCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestUpdateEmail_PostsUserIDAndNewEmail(t *testing.T) {
	var body updateEmailRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if strings.Contains(string(raw), `"current_email"`) || strings.Contains(string(raw), `"expected_email"`) {
			t.Errorf("email fields must be omitted when only --user-id is supplied, got %q", raw)
		}
		testutil.JSON(t, w, map[string]any{
			"message":              "Email change pending confirmation",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.UserID != "2245593582708" || body.ExpectedEmail != "" || body.NewEmail != "new@example.com" {
		t.Errorf("got user_id=%q expected_email=%q new=%q, want only user_id + new_email", body.UserID, body.ExpectedEmail, body.NewEmail)
	}
	if !strings.Contains(out, "User ID: 2245593582708") {
		t.Errorf("when only --user-id is supplied the identifier line must use the User ID label: %q", out)
	}
	if strings.Contains(out, "Current:") {
		t.Errorf("Current label must not appear for user_id targeting: %q", out)
	}
}

func TestUpdateEmail_FallbackHeadlineQualifiesUserID(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":              "",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "user_id 2245593582708 → new@example.com") {
		t.Errorf("fallback headline must qualify the user_id (without this prefix it reads as an email→email change): %q", out)
	}
	if strings.Contains(out, ": 2245593582708 → ") {
		t.Errorf("fallback headline must not place a bare user_id where an email is expected: %q", out)
	}
}

func TestUpdateEmail_ExpectedEmailDoesNotChangeTargetLabel(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":              "",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "old@example.com", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "User ID: 2245593582708") {
		t.Errorf("identifier line must keep the canonical user_id target: %q", out)
	}
	if strings.Contains(out, "Current:") {
		t.Errorf("expected_email must not replace the target label: %q", out)
	}
	if !strings.Contains(out, "Email change pending confirmation: user_id 2245593582708 → new@example.com") {
		t.Errorf("fallback headline must stay anchored to user_id: %q", out)
	}
}

func TestUpdateEmail_ForwardsExpectedEmailAndUserID(t *testing.T) {
	var body updateEmailRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":              "Email change pending confirmation",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--expected-email", "old@example.com",
		"--user-id", "2245593582708",
		"--new-email", "new@example.com",
	})
	testutil.MustExecute(t, cmd)

	if body.ExpectedEmail != "old@example.com" {
		t.Errorf("got expected_email %q, want old@example.com", body.ExpectedEmail)
	}
	if body.UserID != "2245593582708" {
		t.Errorf("got user_id %q, want 2245593582708", body.UserID)
	}
	if body.NewEmail != "new@example.com" {
		t.Errorf("got new_email %q, want new@example.com", body.NewEmail)
	}
}

func TestUpdateEmail_CurrentEmailAndEmailAliasMismatchNamesTypedFlags(t *testing.T) {
	cmd := newUpdateEmailCmd()
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--email", "old@example.com",
		"--current-email", "other@example.com",
		"--new-email", "new@example.com",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected alias mismatch error")
	}
	if !strings.Contains(err.Error(), "--current-email and --email must match") {
		t.Fatalf("got %v, want error naming the two typed aliases", err)
	}
}

func TestUpdateEmail_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestUpdateEmail_PostsUserIDExpectedEmailAndNewEmail(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	var body updateEmailRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":              "Email change pending confirmation. Confirmation email sent to new@example.com.",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "old@example.com", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/update_email" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/update_email", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "old@example.com" || body.NewEmail != "new@example.com" {
		t.Errorf("got user_id=%q expected_email=%q new=%q, want 2245593582708 / old@example.com / new@example.com", body.UserID, body.ExpectedEmail, body.NewEmail)
	}
	if !strings.Contains(out, "Pending: new@example.com") {
		t.Errorf("expected pending email in output: %q", out)
	}
	if !strings.Contains(out, "Confirmed by user: no") {
		t.Errorf("expected pending confirmation status in output: %q", out)
	}
}

func TestUpdateEmail_FallbackHeadlineMatchesPendingConfirmation(t *testing.T) {
	cases := []struct {
		name             string
		pending          bool
		wantHeadline     string
		dontWantHeadline string
	}{
		{
			name:             "pending true uses pending-confirmation framing",
			pending:          true,
			wantHeadline:     "Email change pending confirmation: user_id 2245593582708 → new@example.com",
			dontWantHeadline: "Email change applied:",
		},
		{
			name:             "pending false uses applied framing",
			pending:          false,
			wantHeadline:     "Email change applied: user_id 2245593582708 → new@example.com",
			dontWantHeadline: "Email change pending confirmation:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pending := tc.pending
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				testutil.JSON(t, w, map[string]any{
					"message":              "",
					"unconfirmed_email":    "",
					"pending_confirmation": pending,
				})
			})

			cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.Quiet(false))
			cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if !strings.Contains(out, tc.wantHeadline) {
				t.Errorf("expected fallback headline %q in output: %q", tc.wantHeadline, out)
			}
			if strings.Contains(out, tc.dontWantHeadline) {
				t.Errorf("must not contain %q (contradicts pending_confirmation=%v): %q", tc.dontWantHeadline, tc.pending, out)
			}
		})
	}
}

func TestUpdateEmail_StyledOutputOmitsPendingLineWhenNotPending(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":              "Email change applied",
			"unconfirmed_email":    "",
			"pending_confirmation": false,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Contains(out, "Pending:") {
		t.Errorf("must not print Pending line when pending_confirmation=false (would contradict 'Confirmed by user: yes'), got: %q", out)
	}
	if !strings.Contains(out, "Confirmed by user: yes") {
		t.Errorf("expected confirmed-yes when pending_confirmation=false, got: %q", out)
	}
}

func TestUpdateEmail_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "old@example.com", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/update_email") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	for _, want := range []string{"user_id: 2245593582708", "expected_email: old@example.com", "new_email: new@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in dry-run preview, got: %q", want, out)
		}
	}
}

func TestUpdateEmail_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":              "Email change pending confirmation",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success             bool   `json:"success"`
		UnconfirmedEmail    string `json:"unconfirmed_email"`
		PendingConfirmation bool   `json:"pending_confirmation"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UnconfirmedEmail != "new@example.com" || !resp.PendingConfirmation {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestUpdateEmail_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":              "Email change pending confirmation",
			"unconfirmed_email":    "new@example.com",
			"pending_confirmation": true,
		})
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tEmail change pending confirmation\t2245593582708\tnew@example.com\ttrue"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
