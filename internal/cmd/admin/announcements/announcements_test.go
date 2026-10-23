package announcements

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func campaignJSON(state string) map[string]any {
	return map[string]any{
		"key":            "new_feature",
		"subject":        "Try it",
		"body":           "<p>Hi creator</p>",
		"reply_to":       "support@gumroad.com",
		"audience":       "creators",
		"state":          state,
		"content_digest": "abc123",
	}
}

type recorded struct {
	method, path string
	body         map[string]string
}

func recordRequest(t *testing.T, got *recorded, response map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &got.body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
		}
		testutil.JSON(t, w, response)
	}
}

func run(t *testing.T, cmd interface {
	SetArgs([]string)
	Execute() error
}, args ...string) string {
	t.Helper()
	cmd.SetArgs(args)
	return testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
}

func TestCreatePostsDraftWithBodyFile(t *testing.T) {
	var got recorded
	testutil.SetupAdmin(t, recordRequest(t, &got, map[string]any{"success": true, "campaign": campaignJSON("draft")}))
	bodyFile := filepath.Join(t.TempDir(), "body.html")
	if err := os.WriteFile(bodyFile, []byte("<p>Hi creator</p>"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	out := run(t, cmd, "new_feature", "--audience", "creators", "--subject", "Try it", "--body-file", bodyFile)

	if got.method != http.MethodPost || got.path != "/internal/admin/feature_announcement_campaigns" {
		t.Fatalf("got %s %s", got.method, got.path)
	}
	want := map[string]string{"key": "new_feature", "audience": "creators", "subject": "Try it", "body": "<p>Hi creator</p>", "reply_to": "support@gumroad.com"}
	for k, v := range want {
		if got.body[k] != v {
			t.Fatalf("body[%s] = %q, want %q (%v)", k, got.body[k], v, got.body)
		}
	}
	for _, s := range []string{"Created draft new_feature", "State: draft", "Content digest: abc123"} {
		if !strings.Contains(out, s) {
			t.Fatalf("output missing %q: %q", s, out)
		}
	}
}

func TestCreateRequiresAudienceSubjectAndBody(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"k", "--subject", "s", "--body", "b"}, "--audience"},
		{[]string{"k", "--audience", "creators", "--body", "b"}, "--subject"},
		{[]string{"k", "--audience", "creators", "--subject", "s"}, "--body or --body-file"},
		{[]string{"k", "--audience", "creators", "--subject", "s", "--body", "b", "--body-file", "f"}, "not both"},
	} {
		cmd := newCreateCmd()
		cmd.SetArgs(tc.args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("args %v: got %v, want error with %q", tc.args, err, tc.want)
		}
	}
}

func TestUpdatePutsOnlyChangedFields(t *testing.T) {
	var got recorded
	testutil.SetupAdmin(t, recordRequest(t, &got, map[string]any{"success": true, "campaign": campaignJSON("draft")}))

	run(t, testutil.Command(newUpdateCmd()), "new_feature", "--subject", "New subject")

	if got.method != http.MethodPut || got.path != "/internal/admin/feature_announcement_campaigns/new_feature" {
		t.Fatalf("got %s %s", got.method, got.path)
	}
	if len(got.body) != 1 || got.body["subject"] != "New subject" {
		t.Fatalf("unexpected body: %v", got.body)
	}
}

func TestUpdateRequiresAChange(t *testing.T) {
	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"new_feature"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewPrintsDigestAndBody(t *testing.T) {
	var got recorded
	testutil.SetupAdmin(t, recordRequest(t, &got, map[string]any{
		"success":          true,
		"campaign":         campaignJSON("draft"),
		"subscribed_count": 42,
		"email":            map[string]any{"from": "Gumroad <gumroad@example.com>", "reply_to": []string{"support@gumroad.com"}, "subject": "Try it", "html_body": "<html>\n<p>Rendered copy</p>\n</html>"},
	}))

	out := run(t, testutil.Command(newPreviewCmd(), testutil.Quiet(false)), "new_feature")

	if got.method != http.MethodGet || got.path != "/internal/admin/feature_announcement_campaigns/new_feature/preview" {
		t.Fatalf("got %s %s", got.method, got.path)
	}
	for _, s := range []string{"Content digest: abc123", "Subscribed accounts: 42", "Reply-To: support@gumroad.com", "<p>Rendered copy</p>\n</html>"} {
		if !strings.Contains(out, s) {
			t.Fatalf("output missing %q: %q", s, out)
		}
	}
}

func TestApproveSendsDigest(t *testing.T) {
	var got recorded
	testutil.SetupAdmin(t, recordRequest(t, &got, map[string]any{"success": true, "campaign": campaignJSON("approved")}))

	out := run(t, testutil.Command(newTransitionCmd(approveTransition), testutil.Yes(true), testutil.Quiet(false)), "new_feature", "--content-digest", "abc123")

	if got.method != http.MethodPost || got.path != "/internal/admin/feature_announcement_campaigns/new_feature/approve" {
		t.Fatalf("got %s %s", got.method, got.path)
	}
	if got.body["content_digest"] != "abc123" {
		t.Fatalf("unexpected body: %v", got.body)
	}
	if !strings.Contains(out, "Approved new_feature") || !strings.Contains(out, "State: approved") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestApproveRequiresDigest(t *testing.T) {
	cmd := newTransitionCmd(approveTransition)
	cmd.SetArgs([]string{"new_feature"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--content-digest") {
		t.Fatalf("got %v", err)
	}
}

func TestSendRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not reach API without confirmation")
	})

	cmd := testutil.Command(newTransitionCmd(sendTransition), testutil.NoInput(true))
	cmd.SetArgs([]string{"new_feature"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("got %v", err)
	}
}

func TestSendPostsToDeliver(t *testing.T) {
	var got recorded
	testutil.SetupAdmin(t, recordRequest(t, &got, map[string]any{"success": true, "campaign": campaignJSON("sending")}))

	run(t, testutil.Command(newTransitionCmd(sendTransition), testutil.Yes(true)), "new_feature")

	if got.method != http.MethodPost || got.path != "/internal/admin/feature_announcement_campaigns/new_feature/deliver" {
		t.Fatalf("got %s %s", got.method, got.path)
	}
}

func TestCancelDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not reach the API")
	})

	out := run(t, testutil.Command(newTransitionCmd(cancelTransition), testutil.DryRun(true), testutil.Yes(true)), "new_feature")

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/feature_announcement_campaigns/new_feature/cancel") {
		t.Fatalf("unexpected dry-run output: %q", out)
	}
}

func TestCreateDryRunShowsParams(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not reach the API")
	})

	out := run(t, testutil.Command(newCreateCmd(), testutil.DryRun(true)), "new_feature", "--audience", "creators", "--subject", "Try it", "--body", "<p>Hi</p>")

	for _, s := range []string{"POST", "key: new_feature", "audience: creators"} {
		if !strings.Contains(out, s) {
			t.Fatalf("output missing %q: %q", s, out)
		}
	}
}

func TestViewShowsDeliveryCounts(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":  true,
			"campaign": campaignJSON("sending"),
			"deliveries": []map[string]any{
				{"state": "delivered", "reason": nil, "count": 3},
				{"state": "skipped", "reason": "not_in_audience", "count": 2},
			},
		})
	})

	out := run(t, testutil.Command(newViewCmd(), testutil.Quiet(false)), "new_feature")

	for _, s := range []string{"Deliveries delivered: 3", "Deliveries skipped (not_in_audience): 2"} {
		if !strings.Contains(out, s) {
			t.Fatalf("output missing %q: %q", s, out)
		}
	}
}

func TestListRendersTableAndPlain(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "campaigns": []map[string]any{campaignJSON("sent")}})
	}

	testutil.SetupAdmin(t, handler)
	out := run(t, testutil.Command(newListCmd(), testutil.Quiet(false)))
	if !strings.Contains(out, "KEY") || !strings.Contains(out, "new_feature") {
		t.Fatalf("unexpected table output: %q", out)
	}

	testutil.SetupAdmin(t, handler)
	plain := run(t, testutil.Command(newListCmd(), testutil.PlainOutput()))
	if strings.TrimSpace(plain) != "new_feature\tsent\tcreators\tTry it" {
		t.Fatalf("unexpected plain output: %q", plain)
	}
}

func TestListEmpty(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "campaigns": []map[string]any{}})
	})

	out := run(t, testutil.Command(newListCmd(), testutil.Quiet(false)))
	if !strings.Contains(out, "No campaigns found.") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestJSONOutputPassesThroughResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "campaign": campaignJSON("cancelled")})
	})

	out := run(t, testutil.Command(newTransitionCmd(cancelTransition), testutil.Yes(true), testutil.JSONOutput()), "new_feature")

	var resp campaignResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.Campaign.State != "cancelled" {
		t.Fatalf("unexpected JSON: %s", out)
	}
}

func TestServerErrorSurfaces(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "the copy changed since the preview"})
	})

	cmd := testutil.Command(newTransitionCmd(approveTransition), testutil.Yes(true))
	cmd.SetArgs([]string{"new_feature", "--content-digest", "old"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "the copy changed since the preview") {
		t.Fatalf("got %v", err)
	}
}

func TestNewAnnouncementsCmdRegistersSubcommands(t *testing.T) {
	cmd := NewAnnouncementsCmd()
	names := map[string]bool{}
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"list", "view", "create", "update", "preview", "approve", "send", "cancel"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q", want)
		}
	}
}

func TestViewEscapesControlCharacters(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		c := campaignJSON("draft")
		c["subject"] = "Try it\x1b]52;c;ZXZpbA==\x07"
		testutil.JSON(t, w, map[string]any{"success": true, "campaign": c})
	})

	out := run(t, testutil.Command(newViewCmd(), testutil.Quiet(false)), "new_feature")

	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("output kept control characters: %q", out)
	}
}
