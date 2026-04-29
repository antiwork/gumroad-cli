package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestTwoFactor_NamespaceWiresLeaves(t *testing.T) {
	cmd := newTwoFactorCmd()
	if cmd.Use != "two-factor" {
		t.Fatalf("Use = %q, want two-factor", cmd.Use)
	}

	got := cmd.Commands()
	if len(got) != 2 {
		t.Fatalf("expected 2 leaves, got %d: %#v", len(got), got)
	}
	names := map[string]bool{}
	for _, sub := range got {
		names[sub.Use] = true
	}
	for _, want := range []string{"enable", "disable"} {
		if !names[want] {
			t.Errorf("missing leaf %q in %v", want, names)
		}
	}
}

func TestTwoFactor_EnableRequiresEmail(t *testing.T) {
	cmd := newTwoFactorEnableCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTwoFactor_DisableRequiresEmail(t *testing.T) {
	cmd := newTwoFactorDisableCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTwoFactor_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newTwoFactorDisableCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "user@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestTwoFactor_EnableSendsEnabledTrue(t *testing.T) {
	var gotMethod, gotPath string
	var body twoFactorRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":                           "Two-factor authentication enabled",
			"two_factor_authentication_enabled": true,
		})
	})

	cmd := testutil.Command(newTwoFactorEnableCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/two_factor_authentication" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/two_factor_authentication", gotMethod, gotPath)
	}
	if body.Email != "user@example.com" || !body.Enabled {
		t.Errorf("got email=%q enabled=%v, want user@example.com / true", body.Email, body.Enabled)
	}
	if !strings.Contains(out, "Two-factor: enabled") {
		t.Errorf("expected two-factor state in output: %q", out)
	}
}

func TestTwoFactor_DisableSendsEnabledFalse(t *testing.T) {
	var body twoFactorRequest
	var bodyKeys map[string]json.RawMessage

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.Unmarshal(raw, &bodyKeys); err != nil {
			t.Fatalf("decode body keys: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":                           "Two-factor authentication disabled",
			"two_factor_authentication_enabled": false,
		})
	})

	cmd := testutil.Command(newTwoFactorDisableCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.Enabled {
		t.Errorf("expected enabled=false in body, got %#v", body)
	}
	if _, present := bodyKeys["enabled"]; !present {
		t.Errorf("enabled must always be present (false is meaningful), got body keys: %v", bodyKeys)
	}
	if !strings.Contains(out, "Two-factor: disabled") {
		t.Errorf("expected disabled state in output: %q", out)
	}
}

func TestTwoFactor_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newTwoFactorDisableCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/two_factor_authentication") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	if !strings.Contains(out, "enabled: false") {
		t.Errorf("expected enabled=false in dry-run preview, got: %q", out)
	}
}

func TestTwoFactor_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                           "Two-factor authentication disabled",
			"two_factor_authentication_enabled": false,
		})
	})

	cmd := testutil.Command(newTwoFactorDisableCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool `json:"success"`
		Enabled bool `json:"two_factor_authentication_enabled"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Enabled {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestTwoFactor_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                           "Two-factor authentication enabled",
			"two_factor_authentication_enabled": true,
		})
	})

	cmd := testutil.Command(newTwoFactorEnableCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tTwo-factor authentication enabled\tuser@example.com\tenabled"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
