package payouts

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestResume_RequiresEmail(t *testing.T) {
	cmd := newResumeCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResume_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newResumeCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestResume_SendsEmailOnly(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body resumeRequest

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
		testutil.JSON(t, w, map[string]any{
			"message":        "Payouts resumed for seller@example.com",
			"payouts_paused": false,
		})
	})

	cmd := testutil.Command(newResumeCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/payouts/resume" {
		t.Fatalf("got %s %s, want POST /internal/admin/payouts/resume", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", body.Email)
	}
	for _, want := range []string{"Payouts resumed for seller@example.com", "Email: seller@example.com", "Payouts: resumed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestResume_NotPausedShortCircuit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":         "not_paused",
			"message":        "Payouts are not paused",
			"payouts_paused": false,
		})
	})

	cmd := testutil.Command(newResumeCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Payouts are not paused", "Status: not_paused", "Payouts: resumed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestResume_DryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the resume endpoint")
	})

	cmd := testutil.Command(newResumeCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/payouts/resume") {
		t.Errorf("expected dry-run preview to mention POST and the resume path, got: %q", out)
	}
	if !strings.Contains(out, "email: seller@example.com") {
		t.Errorf("expected dry-run preview to include email, got: %q", out)
	}
}

func TestResume_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":         "not_paused",
			"message":        "Payouts are not paused",
			"payouts_paused": false,
		})
	})

	cmd := testutil.Command(newResumeCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp payoutsActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "not_paused" || resp.Message != "Payouts are not paused" || resp.PayoutsPaused {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestResume_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":        "Payouts resumed for seller@example.com",
			"payouts_paused": false,
		})
	})

	cmd := testutil.Command(newResumeCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tPayouts resumed for seller@example.com\tseller@example.com\t\tresumed"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
