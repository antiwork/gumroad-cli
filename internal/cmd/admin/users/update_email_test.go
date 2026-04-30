package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUpdateEmail_RequiresBothFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing-current", []string{"--new-email", "new@example.com"}, "missing required flag: --current-email"},
		{"missing-new", []string{"--current-email", "old@example.com"}, "missing required flag: --new-email"},
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

func TestUpdateEmail_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestUpdateEmail_PostsBothEmails(t *testing.T) {
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
	cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/update_email" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/update_email", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("emails must not appear in query string, got %q", gotQuery)
	}
	if body.CurrentEmail != "old@example.com" || body.NewEmail != "new@example.com" {
		t.Errorf("got current=%q new=%q, want old@example.com / new@example.com", body.CurrentEmail, body.NewEmail)
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
			wantHeadline:     "Email change pending confirmation: old@example.com → new@example.com",
			dontWantHeadline: "Email change applied:",
		},
		{
			name:             "pending false uses applied framing",
			pending:          false,
			wantHeadline:     "Email change applied: old@example.com → new@example.com",
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
			cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})
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
	cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})
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
	cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/update_email") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	for _, want := range []string{"current_email: old@example.com", "new_email: new@example.com"} {
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
	cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})
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
	cmd.SetArgs([]string{"--current-email", "old@example.com", "--new-email", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tEmail change pending confirmation\told@example.com\tnew@example.com\ttrue"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
