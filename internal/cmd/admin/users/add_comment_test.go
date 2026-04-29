package users

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestAddComment_RequiresEmailAndContent(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing-email", []string{"--content", "x"}, "missing required flag: --email"},
		{"missing-content", []string{"--email", "user@example.com"}, "missing required flag: --content"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newAddCommentCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestAddComment_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "user@example.com", "--content", "test note"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestAddComment_AutoGeneratesIdempotencyKey(t *testing.T) {
	var body addCommentRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"comment": map[string]any{
				"id":           "c_1",
				"author_name":  "Admin",
				"content":      "test note",
				"comment_type": "Note",
				"created_at":   "2026-04-24T12:00:00Z",
			},
		})
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--email", "user@example.com", "--content", "test note"})
	testutil.MustExecute(t, cmd)

	if body.Email != "user@example.com" || body.Content != "test note" {
		t.Errorf("got email=%q content=%q, want forwarded values", body.Email, body.Content)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(body.IdempotencyKey) {
		t.Errorf("expected auto-generated 32-char hex idempotency key, got %q", body.IdempotencyKey)
	}
}

func TestAddComment_ForwardsExplicitIdempotencyKey(t *testing.T) {
	var body addCommentRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"comment": map[string]any{"id": "c_1", "comment_type": "Note"},
		})
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--email", "user@example.com",
		"--content", "test note",
		"--idempotency-key", "fixed-key-1",
	})
	testutil.MustExecute(t, cmd)

	if body.IdempotencyKey != "fixed-key-1" {
		t.Errorf("got idempotency_key %q, want fixed-key-1", body.IdempotencyKey)
	}
}

func TestAddComment_PostsToCorrectPath(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"comment": map[string]any{"id": "c_1", "comment_type": "Note", "created_at": "2026-04-24T12:00:00Z"},
		})
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "user@example.com", "--content", "test note"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/create_comment" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/create_comment", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("body must not appear in query string, got %q", gotQuery)
	}
	if !strings.Contains(out, "Comment ID: c_1") {
		t.Errorf("expected comment ID in output: %q", out)
	}
}

func TestAddComment_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{
		"--email", "user@example.com",
		"--content", "test note",
		"--idempotency-key", "fixed-key-1",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/create_comment") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	for _, want := range []string{"email: user@example.com", "content: test note", "idempotency_key: fixed-key-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in dry-run preview, got: %q", want, out)
		}
	}
}

func TestAddComment_DryRunPlaceholderForAutoGeneratedKey(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{
		"--email", "user@example.com",
		"--content", "test note",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "idempotency_key: <auto-generated, regenerated on real call>") {
		t.Errorf("expected placeholder in dry-run preview when --idempotency-key is omitted, got: %q", out)
	}
	if regexp.MustCompile(`idempotency_key:\s*[0-9a-f]{32}`).MatchString(out) {
		t.Errorf("dry-run preview must not show a concrete auto-generated hex key, got: %q", out)
	}
}

func TestAddComment_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"comment": map[string]any{
				"id":           "c_1",
				"comment_type": "Note",
				"content":      "test note",
				"created_at":   "2026-04-24T12:00:00Z",
			},
		})
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "user@example.com", "--content", "test note"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool           `json:"success"`
		Comment map[string]any `json:"comment"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Comment["id"] != "c_1" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestAddComment_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"comment": map[string]any{
				"id":           "c_1",
				"comment_type": "Note",
				"content":      "test note",
				"created_at":   "2026-04-24T12:00:00Z",
			},
		})
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "user@example.com", "--content", "test note"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tAdded admin note to user@example.com\tuser@example.com\tc_1\tNote\t2026-04-24T12:00:00Z"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestAddComment_ConflictSurfacesIdempotencyError(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Idempotency key already used with different content",
		})
	})

	cmd := testutil.Command(newAddCommentCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--email", "user@example.com",
		"--content", "different content",
		"--idempotency-key", "reused-key",
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Idempotency key already used") {
		t.Fatalf("expected idempotency conflict to surface, got: %v", err)
	}
}
