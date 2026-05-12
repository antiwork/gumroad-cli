package comments

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestListUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotEmail, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"user_id":    "user_123",
			"comments":   []map[string]any{commentFixture()},
			"pagination": map[string]any{"next": nil, "limit": 20},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/comments" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/comments", gotMethod, gotPath)
	}
	if gotEmail != "user@example.com" {
		t.Fatalf("got email %q, want user@example.com", gotEmail)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{
		"1 comment(s) for user@example.com",
		"User ID: user_123",
		"c_123 Note",
		"Author: Admin User",
		"Created: 2026-05-01T12:00:00Z",
		"Customer said the receipt was missing",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestListPassesUserIDTypesLimitAndCursor(t *testing.T) {
	var gotQuery string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"comments":   []any{},
			"pagination": map[string]any{"next": nil, "limit": 50},
			"user_id":    "user_123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{
		"--user-id", "user_123",
		"--type", "Note",
		"--type", "Refund_note",
		"--limit", "50",
		"--cursor", "cur-1",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"user_id=user_123", "comment_type=Note%2CRefund_note", "limit=50", "cursor=cur-1"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %q: %q", want, gotQuery)
		}
	}
	if !strings.Contains(out, "No comments found for user_123.") {
		t.Fatalf("unexpected empty output: %q", out)
	}
}

func TestListShowsNextCursorFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"comments": []map[string]any{
				commentFixture(),
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 1},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "More results: --cursor cur-next") {
		t.Fatalf("expected next-cursor footer, got: %q", out)
	}
}

func TestListRequiresEmailOrUserID(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email or --user-id") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestListRejectsInvalidLimit(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{"--user-id", "user_123", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected zero limit error, got %v", err)
	}
}

func TestListJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"user_id": "user_123",
			"comments": []map[string]any{
				commentFixture(),
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool             `json:"success"`
		UserID     string           `json:"user_id"`
		Comments   []map[string]any `json:"comments"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "user_123" {
		t.Fatalf("unexpected JSON envelope: %s", out)
	}
	if len(resp.Comments) != 1 || resp.Comments[0]["id"] != "c_123" {
		t.Fatalf("unexpected JSON comments: %s", out)
	}
	if resp.Pagination["next"] != "cur-next" {
		t.Fatalf("unexpected pagination: %s", out)
	}
}

func TestListPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"comments": []map[string]any{
				commentFixture(),
				{
					"id":           "c_456",
					"comment_type": "Refund_note",
					"author":       map[string]any{"name": "Support", "email": "support@example.com"},
					"content":      "Refunded after duplicate charge\nsecond line",
					"created_at":   "2026-05-02T12:00:00Z",
					"deleted_at":   "2026-05-03T12:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "user@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := strings.Join([]string{
		"c_123\tNote\tAdmin User\t2026-05-01T12:00:00Z\t\tCustomer said the receipt was missing.",
		"c_456\tRefund_note\tSupport / support@example.com\t2026-05-02T12:00:00Z\t2026-05-03T12:00:00Z\tRefunded after duplicate charge\\nsecond line",
	}, "\n")
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestListHumanOutputTruncatesContentAndMarksDeleted(t *testing.T) {
	longContent := strings.Repeat("a", commentPreviewLimit+1)
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"comments": []map[string]any{
				{
					"id":           "c_deleted",
					"comment_type": "Refund_note",
					"author_name":  "Admin User",
					"content":      longContent,
					"created_at":   "2026-05-01T12:00:00Z",
					"deleted_at":   "2026-05-02T12:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "c_deleted Refund_note (deleted)") {
		t.Fatalf("expected deleted marker, got %q", out)
	}
	if !strings.Contains(out, "Deleted: 2026-05-02T12:00:00Z") {
		t.Fatalf("expected deleted_at line, got %q", out)
	}
	wantPreview := strings.Repeat("a", commentPreviewLimit) + "..."
	if !strings.Contains(out, wantPreview) {
		t.Fatalf("expected truncated preview, got %q", out)
	}
	if strings.Contains(out, longContent) {
		t.Fatalf("human output should truncate long content, got %q", out)
	}
}

func TestNewCommentsCmdWiresSubcommands(t *testing.T) {
	cmd := NewCommentsCmd()
	if cmd.Use != "comments" {
		t.Fatalf("Use = %q, want comments", cmd.Use)
	}

	got := cmd.Commands()
	want := []string{"list", "add"}

	if len(got) != len(want) {
		t.Fatalf("expected %d subcommands, got %d: %#v", len(want), len(got), got)
	}

	names := map[string]bool{}
	for _, sub := range got {
		names[sub.Use] = true
	}
	for _, name := range want {
		if !names[name] {
			t.Errorf("missing subcommand %q in %v", name, names)
		}
	}
}

func commentFixture() map[string]any {
	return map[string]any{
		"id":           "c_123",
		"comment_type": "Note",
		"author_name":  "Admin User",
		"content":      "Customer said the receipt was missing.",
		"created_at":   "2026-05-01T12:00:00Z",
		"deleted_at":   nil,
	}
}
