package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestMarkCompliantRequiresEmailOrExternalID(t *testing.T) {
	cmd := newMarkCompliantCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "supply --email or --external-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkCompliantSendsExternalID(t *testing.T) {
	var body markCompliantRequest

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
			"status":  "marked_compliant",
			"message": "User marked compliant",
		})
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--external-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.ExternalID != "2245593582708" || body.Email != "" {
		t.Errorf("got email=%q external_id=%q, want only external_id", body.Email, body.ExternalID)
	}
	if !strings.Contains(out, "External ID: 2245593582708") {
		t.Errorf("expected External ID label when only --external-id is supplied: %q", out)
	}
}

func TestMarkCompliantForwardsBothEmailAndExternalID(t *testing.T) {
	var body markCompliantRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "marked_compliant",
			"message": "User marked compliant",
		})
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--email", "seller@example.com",
		"--external-id", "2245593582708",
	})
	testutil.MustExecute(t, cmd)

	if body.Email != "seller@example.com" || body.ExternalID != "2245593582708" {
		t.Errorf("got email=%q external_id=%q, want both forwarded", body.Email, body.ExternalID)
	}
}

func TestMarkCompliantRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestMarkCompliantSendsEmailAndNote(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var body markCompliantRequest

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
			"status":  "marked_compliant",
			"message": "User marked compliant",
		})
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--note", "Cleared after review"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/mark_compliant" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/mark_compliant", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("email/note must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.Email != "seller@example.com" || body.Note != "Cleared after review" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User marked compliant", "Email: seller@example.com", "Status: marked_compliant"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestMarkCompliantDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the mark_compliant endpoint")
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--note", "Retry"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/mark_compliant") {
		t.Errorf("expected dry-run preview to mention POST and the mark_compliant path, got: %q", out)
	}
	if !strings.Contains(out, "email: seller@example.com") || !strings.Contains(out, "note: Retry") {
		t.Errorf("expected dry-run preview to include email and note, got: %q", out)
	}
}

func TestMarkCompliantJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "already_compliant",
			"message": "User is already compliant",
		})
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp riskActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "already_compliant" || resp.Message != "User is already compliant" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestMarkCompliantPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "marked_compliant",
			"message": "User marked compliant",
		})
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tUser marked compliant\tseller@example.com\tmarked_compliant"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
