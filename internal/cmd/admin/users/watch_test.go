package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func sampleWatchPayload(message string) map[string]any {
	return map[string]any{
		"success": true,
		"message": message,
		"watched_user": map[string]any{
			"id":                      "watch_123",
			"revenue_threshold_cents": 20000,
			"revenue_cents":           0,
			"unpaid_balance_cents":    2500,
			"notes":                   "Check next independent buyers",
			"created_at":              "2026-05-01T10:00:00Z",
			"last_synced_at":          "2026-05-06T12:00:00Z",
		},
	}
}

func TestWatchRequiresUserID(t *testing.T) {
	cmd := newWatchCmd()
	cmd.SetArgs([]string{"--revenue-threshold", "200"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing user ID error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWatchRequiresRevenueThreshold(t *testing.T) {
	cmd := newWatchCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing revenue threshold error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --revenue-threshold") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWatchRejectsNonPositiveRevenueThreshold(t *testing.T) {
	cmd := newWatchCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--revenue-threshold", "0"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid revenue threshold error")
	}
	if !strings.Contains(err.Error(), "--revenue-threshold must be greater than 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWatchRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newWatchCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--revenue-threshold", "200"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestWatchSendsUserIDExpectedEmailThresholdAndNote(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var body watchRequest

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
		testutil.JSON(t, w, sampleWatchPayload("User added to watchlist"))
	})

	cmd := testutil.Command(newWatchCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--expected-email", "seller@example.com",
		"--revenue-threshold", "200",
		"--note", "Check next independent buyers",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/watch" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/watch", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || body.RevenueThreshold != "200.00" || body.Notes != "Check next independent buyers" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{
		"User added to watchlist",
		"User ID: 2245593582708",
		"Watch ID: watch_123",
		"Revenue: $0.00 of $200.00",
		"Unpaid balance: $25.00",
		"Note: Check next independent buyers",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestWatchDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the watch endpoint")
	})

	cmd := testutil.Command(newWatchCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com", "--revenue-threshold", "200", "--note", "Review later"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/watch") {
		t.Errorf("expected dry-run preview to mention POST and the watch path, got: %q", out)
	}
	for _, want := range []string{"user_id: 2245593582708", "expected_email: seller@example.com", "revenue_threshold: 200.00", "notes: Review later"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run preview missing %q: %q", want, out)
		}
	}
}

func TestWatchPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleWatchPayload("User added to watchlist"))
	})

	cmd := testutil.Command(newWatchCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--revenue-threshold", "200"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tUser added to watchlist\t2245593582708\twatch_123\t20000\t0\t2500\tCheck next independent buyers\t2026-05-01T10:00:00Z\t2026-05-06T12:00:00Z"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestUpdateWatchPreservesNoteWhenOmitted(t *testing.T) {
	var body updateWatchRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/internal/admin/users/update_watch" {
			t.Fatalf("got %s %s, want POST /internal/admin/users/update_watch", r.Method, r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, sampleWatchPayload("Watchlist updated"))
	})

	cmd := testutil.Command(newUpdateWatchCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--revenue-threshold", "500"})
	testutil.MustExecute(t, cmd)

	if body.UserID != "2245593582708" || body.RevenueThreshold != "500.00" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	if body.Notes != nil {
		t.Fatalf("notes must be omitted when --note is omitted, got %#v", *body.Notes)
	}
}

func TestUpdateWatchCanClearNote(t *testing.T) {
	var body updateWatchRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, sampleWatchPayload("Watchlist updated"))
	})

	cmd := testutil.Command(newUpdateWatchCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--revenue-threshold", "500", "--clear-note"})
	testutil.MustExecute(t, cmd)

	if body.Notes == nil {
		t.Fatal("notes must be sent when --clear-note is set")
	}
	if *body.Notes != "" {
		t.Fatalf("got notes %q, want empty string", *body.Notes)
	}
}

func TestUpdateWatchRejectsNoteAndClearNote(t *testing.T) {
	cmd := newUpdateWatchCmd()
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--revenue-threshold", "500",
		"--note", "Keep this",
		"--clear-note",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected mutually exclusive note error")
	}
	if !strings.Contains(err.Error(), "--note and --clear-note cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnwatchSendsUserID(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var body unwatchRequest

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
			"message": "User removed from watchlist",
		})
	})

	cmd := testutil.Command(newUnwatchCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/unwatch" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/unwatch", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User removed from watchlist", "User ID: 2245593582708"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestUnwatchDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the unwatch endpoint")
	})

	cmd := testutil.Command(newUnwatchCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/unwatch") {
		t.Errorf("expected dry-run preview to mention POST and the unwatch path, got: %q", out)
	}
	if !strings.Contains(out, "user_id: 2245593582708") {
		t.Errorf("dry-run preview missing user_id: %q", out)
	}
}
