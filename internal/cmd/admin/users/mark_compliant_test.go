package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestMarkCompliantRequiresUserID(t *testing.T) {
	cmd := newMarkCompliantCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing identifier error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkCompliantSendsUserID(t *testing.T) {
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
			t.Errorf("email field must be omitted when only --user-id is supplied, got %q", raw)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"status":  "marked_compliant",
			"message": "User marked compliant",
		})
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if body.UserID != "2245593582708" || body.ExpectedEmail != "" {
		t.Errorf("got user_id=%q expected_email=%q, want only user_id", body.UserID, body.ExpectedEmail)
	}
	if !strings.Contains(out, "User ID: 2245593582708") {
		t.Errorf("expected User ID label when only --user-id is supplied: %q", out)
	}
}

func TestMarkCompliantForwardsExpectedEmail(t *testing.T) {
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
		"--user-id", "2245593582708",
		"--expected-email", "seller@example.com",
	})
	testutil.MustExecute(t, cmd)

	if body.ExpectedEmail != "seller@example.com" || body.UserID != "2245593582708" {
		t.Errorf("got expected_email=%q user_id=%q, want both forwarded", body.ExpectedEmail, body.UserID)
	}
}

func TestMarkCompliantRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newMarkCompliantCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestMarkCompliantSendsUserIDExpectedEmailAndNote(t *testing.T) {
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
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--note", "Cleared after review"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/mark_compliant" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/mark_compliant", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || body.Note != "Cleared after review" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User marked compliant", "User ID: 2245593582708", "Status: marked_compliant"} {
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
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--note", "Retry"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/mark_compliant") {
		t.Errorf("expected dry-run preview to mention POST and the mark_compliant path, got: %q", out)
	}
	if !strings.Contains(out, "user_id: 2245593582708") || !strings.Contains(out, "expected_email: seller@example.com") || !strings.Contains(out, "note: Retry") {
		t.Errorf("expected dry-run preview to include user_id, expected_email, and note, got: %q", out)
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
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
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
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tUser marked compliant\t2245593582708\tmarked_compliant"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
