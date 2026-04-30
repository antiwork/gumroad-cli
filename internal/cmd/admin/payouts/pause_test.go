package payouts

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPause_RequiresEmail(t *testing.T) {
	cmd := newPauseCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPause_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newPauseCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestPause_OmitsReasonWhenAbsent(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body pauseRequest
	var bodyKeys map[string]json.RawMessage

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
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
			"message":        "Payouts paused for seller@example.com",
			"payouts_paused": true,
		})
	})

	cmd := testutil.Command(newPauseCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/payouts/pause" {
		t.Fatalf("got %s %s, want POST /internal/admin/payouts/pause", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email/reason must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", body.Email)
	}
	if _, present := bodyKeys["reason"]; present {
		t.Errorf("reason must be omitted when not set, got body keys: %v", bodyKeys)
	}
	for _, want := range []string{"Payouts paused for seller@example.com", "Email: seller@example.com", "Payouts: paused"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestPause_ForwardsReason(t *testing.T) {
	var body pauseRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":        "Payouts paused for seller@example.com",
			"payouts_paused": true,
		})
	})

	cmd := testutil.Command(newPauseCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--reason", "Verification pending"})
	testutil.MustExecute(t, cmd)

	if body.Reason != "Verification pending" {
		t.Errorf("expected reason=%q, got %#v", "Verification pending", body)
	}
}

func TestPause_AlreadyPausedShortCircuit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":         "already_paused",
			"message":        "Payouts are already paused",
			"payouts_paused": true,
		})
	})

	cmd := testutil.Command(newPauseCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Payouts are already paused", "Status: already_paused", "Payouts: paused"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestPause_DryRunIncludesReason(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the pause endpoint")
	})

	cmd := testutil.Command(newPauseCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--reason", "Verification pending"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/payouts/pause") {
		t.Errorf("expected dry-run preview to mention POST and the pause path, got: %q", out)
	}
	if !strings.Contains(out, "email: seller@example.com") || !strings.Contains(out, "reason: Verification pending") {
		t.Errorf("expected dry-run preview to include email and reason, got: %q", out)
	}
}

func TestPause_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":         "already_paused",
			"message":        "Payouts are already paused",
			"payouts_paused": true,
		})
	})

	cmd := testutil.Command(newPauseCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp payoutsActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "already_paused" || resp.Message != "Payouts are already paused" || !resp.PayoutsPaused {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestPause_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":        "Payouts paused for seller@example.com",
			"payouts_paused": true,
		})
	})

	cmd := testutil.Command(newPauseCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tPayouts paused for seller@example.com\tseller@example.com\t\tpaused"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestPause_UserNotFoundSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "User not found",
		})
	})

	cmd := testutil.Command(newPauseCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--email", "missing@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected user-not-found error")
	}
	if !strings.Contains(err.Error(), "User not found") {
		t.Errorf("missing underlying message: %v", err)
	}
}
