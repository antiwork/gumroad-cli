package emails

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	_ "unsafe"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

//go:linkname promptIsTerminal github.com/antiwork/gumroad-cli/internal/prompt.isTerminal
var promptIsTerminal func(int) bool

const (
	emailBodyFileMode       = 0600
	emailTestAudienceCount  = 12
	emailTestRecipientCount = 10
)

func writeEmailBody(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "body.html")
	if err := os.WriteFile(path, []byte(body), emailBodyFileMode); err != nil {
		t.Fatalf("write body: %v", err)
	}
	return path
}

func emailPayload(id, subject, state string) map[string]any {
	return map[string]any{
		"id":               id,
		"subject":          subject,
		"message":          "<p>Hello</p>",
		"audience_type":    "audience",
		"state":            state,
		"published_at":     "2026-06-17T10:00:00Z",
		"scheduled_at":     "",
		"send_emails":      true,
		"url":              "https://example.com/emails/" + id,
		"audience_count":   emailTestAudienceCount,
		"recipients_count": emailTestRecipientCount,
		"created_at":       "2026-06-17T09:00:00Z",
		"updated_at":       "2026-06-17T09:30:00Z",
	}
}

func completeEmailPayload(id, subject, state string) map[string]any {
	item := emailPayload(id, subject, state)
	item["product_id"] = ""
	item["shown_on_profile"] = false
	return item
}

func declinedConfirmationInput(t *testing.T) *os.File {
	t.Helper()

	oldIsTerminal := promptIsTerminal
	promptIsTerminal = func(int) bool { return true }
	t.Cleanup(func() { promptIsTerminal = oldIsTerminal })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe declined confirmation: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("write declined confirmation: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close declined confirmation writer: %v", err)
	}

	return r
}

func assertUsageError(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected usage error")
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("got %T, want *cmdutil.UsageError", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestNewEmailsCmd_HelpMentionsDraftPreviewWorkflow(t *testing.T) {
	cmd := NewEmailsCmd()
	if !strings.Contains(cmd.Long, "created as drafts by default") {
		t.Fatalf("expected draft default in help, got %q", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "gumroad emails send-preview <id>") {
		t.Fatalf("expected preview workflow in help, got %q", cmd.Long)
	}
}

func TestCreate_DefaultDraftPostsBodyFile(t *testing.T) {
	bodyPath := writeEmailBody(t, "<h1>Launch</h1>")
	var gotMethod, gotPath string
	var gotForm url.Values

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", r.PostForm.Get("subject"), "draft")})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--subject", "Launch", "--body", bodyPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/emails" {
		t.Fatalf("got %s %s, want POST /emails", gotMethod, gotPath)
	}
	if gotForm.Get("subject") != "Launch" {
		t.Fatalf("subject = %q, want Launch", gotForm.Get("subject"))
	}
	if gotForm.Get("body") != "<h1>Launch</h1>" {
		t.Fatalf("body = %q", gotForm.Get("body"))
	}
	if gotForm.Get("audience") != "audience" {
		t.Fatalf("audience = %q, want audience", gotForm.Get("audience"))
	}
	if _, ok := gotForm["publish"]; ok {
		t.Fatal("publish must be omitted for default draft creation")
	}
	if !strings.Contains(out, "Created email:") || !strings.Contains(out, "Launch") || !strings.Contains(out, "email_123") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCreate_SendPublishesWithoutDraftParam(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Now</p>")
	var gotForm url.Values

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Now", "published")})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--subject", "Now", "--body", bodyPath, "--send"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotForm.Get("publish") != "true" {
		t.Fatalf("publish = %q, want true", gotForm.Get("publish"))
	}
	if _, ok := gotForm["draft"]; ok {
		t.Fatal("draft param must never be sent")
	}
}

func TestCreate_BodyFromStdin(t *testing.T) {
	var gotBody string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotBody = r.PostForm.Get("body")
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Piped", "draft")})
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString("<p>From stdin</p>"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	cmd := testutil.Command(newCreateCmd(), testutil.Stdin(r))
	cmd.SetArgs([]string{"--subject", "Piped", "--body", "-"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotBody != "<p>From stdin</p>" {
		t.Fatalf("body = %q, want stdin contents", gotBody)
	}
}

func TestCreate_InvalidAudienceReturnsUsageError(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Hello</p>")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called for invalid audience")
	})

	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--subject", "Hello", "--body", bodyPath, "--audience", "buyers"})
	err := cmd.Execute()

	assertUsageError(t, err, "--audience must be one of: all, customers, followers, product")
}

func TestCreate_ProductAudienceRequiresProduct(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Hello</p>")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called without product")
	})

	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--subject", "Hello", "--body", bodyPath, "--audience", "product"})
	err := cmd.Execute()

	assertUsageError(t, err, "--product")
}

func TestCreate_ProductFlagRequiresProductAudience(t *testing.T) {
	cases := []struct {
		name         string
		audienceArgs []string
	}{
		{name: "default audience"},
		{name: "customers audience", audienceArgs: []string{"--audience", "customers"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bodyPath := writeEmailBody(t, "<p>Hello</p>")
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("API must not be called when product audience is not selected")
			})

			args := []string{"--subject", "Hello", "--body", bodyPath}
			args = append(args, tc.audienceArgs...)
			args = append(args, "--product", "prod_123")
			cmd := testutil.Command(newCreateCmd())
			cmd.SetArgs(args)
			err := cmd.Execute()

			assertUsageError(t, err, "--product requires --audience product")
		})
	}
}

func TestCreate_ProductAudienceSendsLinkID(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Product update</p>")
	var gotAudience string
	var gotLinkID string

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotAudience = r.PostForm.Get("audience")
		gotLinkID = r.PostForm.Get("link_id")
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Product update", "draft")})
	})

	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--subject", "Product update", "--body", bodyPath, "--audience", "product", "--product", "prod_123"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotAudience != "product" {
		t.Fatalf("audience = %q, want product", gotAudience)
	}
	if gotLinkID != "prod_123" {
		t.Fatalf("link_id = %q, want prod_123", gotLinkID)
	}
}

func TestCreate_MapsAudienceLabelsToAPIValues(t *testing.T) {
	cases := []struct {
		name         string
		audience     string
		wantAudience string
	}{
		{name: "all", audience: "all", wantAudience: "audience"},
		{name: "customers", audience: "customers", wantAudience: "seller"},
		{name: "followers", audience: "followers", wantAudience: "follower"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bodyPath := writeEmailBody(t, "<p>Segment</p>")
			var gotAudience string

			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm failed: %v", err)
				}
				gotAudience = r.PostForm.Get("audience")
				testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Segment", "draft")})
			})

			cmd := testutil.Command(newCreateCmd())
			cmd.SetArgs([]string{"--subject", "Segment", "--body", bodyPath, "--audience", tc.audience})
			testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if gotAudience != tc.wantAudience {
				t.Fatalf("audience = %q, want %s", gotAudience, tc.wantAudience)
			}
		})
	}
}

func TestCreate_MissingBodyFileReturnsUsageError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called when body file is missing")
	})

	missing := filepath.Join(t.TempDir(), "missing.html")
	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--subject", "Missing", "--body", missing})
	err := cmd.Execute()

	assertUsageError(t, err, "--body: cannot read")
}

func TestCreate_DryRunPrintsRequestWithoutCallingAPI(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Preview params</p>")
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("API must not be called during dry-run")
	})

	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--subject", "Preview params", "--body", bodyPath, "--audience", "followers"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if called {
		t.Fatal("API was called during dry-run")
	}
	for _, want := range []string{"POST", "/emails", "subject: Preview params", "audience: follower", "body: <p>Preview params</p>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q in %q", want, out)
		}
	}
}

func TestPreview_PostsEndpointAndPrintsURL(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"preview_url": "https://example.com/preview/email_123",
			"message":     "Preview sent to your email.",
		})
	})

	cmd := testutil.Command(newSendPreviewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/emails/email_123/preview" {
		t.Fatalf("got %s %s, want POST /emails/email_123/preview", gotMethod, gotPath)
	}
	if !strings.Contains(out, "https://example.com/preview/email_123") {
		t.Fatalf("output missing preview URL: %q", out)
	}
}

func TestPreview_DefaultOutputPrintsFallbackMessageAndURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"preview_url": "https://example.com/preview/email_123",
			"message":     "",
		})
	})

	cmd := testutil.Command(newSendPreviewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Preview sent to your email.", "https://example.com/preview/email_123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("preview output missing %q in %q", want, out)
		}
	}
}

func TestPreview_PlainOutputPrintsURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"preview_url": "https://example.com/preview/email_123",
			"message":     "Preview sent to your email.",
		})
	})

	cmd := testutil.Command(newSendPreviewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "https://example.com/preview/email_123\n" {
		t.Fatalf("got %q, want plain preview URL", out)
	}
}

func TestList_RendersRowsAndSendsStateType(t *testing.T) {
	var gotQuery url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"emails": []map[string]any{
				emailPayload("email_123", "Draft note", "draft"),
			},
		})
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--state", "draft"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotQuery.Get("type") != "draft" {
		t.Fatalf("type = %q, want draft", gotQuery.Get("type"))
	}
	if !strings.Contains(out, "email_123") || !strings.Contains(out, "Draft note") || !strings.Contains(out, "draft") {
		t.Fatalf("list output missing row: %q", out)
	}
}

func TestList_PlainOutputRendersRowsWithDisplayDates(t *testing.T) {
	scheduled := completeEmailPayload("email_scheduled", "Scheduled note", "scheduled")
	scheduled["published_at"] = ""
	scheduled["scheduled_at"] = "2026-06-18T14:00:00Z"

	draft := completeEmailPayload("email_draft", "Draft note", "draft")
	draft["published_at"] = ""
	draft["scheduled_at"] = ""

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"emails": []map[string]any{
				completeEmailPayload("email_published", "Published note", "published"),
				scheduled,
				draft,
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"email_published\tPublished note\tpublished\tall\t\t2026-06-17T10:00:00Z\n",
		"email_scheduled\tScheduled note\tscheduled\tall\t\t2026-06-18T14:00:00Z\n",
		"email_draft\tDraft note\tdraft\tall\t\t\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain list output missing %q in %q", want, out)
		}
	}
}

func TestEmailDisplayDateUsesStateSpecificTimestamp(t *testing.T) {
	cases := []struct {
		name string
		item emailRecord
		want string
	}{
		{
			name: "published",
			item: emailRecord{State: emailStatePublished, PublishedAt: "2026-06-17T10:00:00Z", ScheduledAt: "2026-06-18T14:00:00Z"},
			want: "2026-06-17T10:00:00Z",
		},
		{
			name: "scheduled",
			item: emailRecord{State: emailStateScheduled, PublishedAt: "2026-06-17T10:00:00Z", ScheduledAt: "2026-06-18T14:00:00Z"},
			want: "2026-06-18T14:00:00Z",
		},
		{
			name: "draft",
			item: emailRecord{State: emailStateDraft, PublishedAt: "2026-06-17T10:00:00Z", ScheduledAt: "2026-06-18T14:00:00Z"},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailDisplayDate(tc.item); got != tc.want {
				t.Fatalf("emailDisplayDate() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestList_RendersFriendlyAudiencesAndProductTargets(t *testing.T) {
	all := completeEmailPayload("email_all", "All buyers", "draft")

	customers := completeEmailPayload("email_customers", "Customers", "draft")
	customers["audience_type"] = "seller"

	followers := completeEmailPayload("email_followers", "Followers", "draft")
	followers["audience_type"] = "follower"

	product := completeEmailPayload("email_product", "Product", "draft")
	product["audience_type"] = "product"
	product["product_id"] = "prod_123"

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"emails": []map[string]any{all, customers, followers, product},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"email_all\tAll buyers\tdraft\tall\t\t\n",
		"email_customers\tCustomers\tdraft\tcustomers\t\t\n",
		"email_followers\tFollowers\tdraft\tfollowers\t\t\n",
		"email_product\tProduct\tdraft\tproduct\tprod_123\t\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain list output missing %q in %q", want, out)
		}
	}
	for _, unwanted := range []string{"\tseller\t", "\tfollower\t", "\taudience\t"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("plain list output leaked server audience %q in %q", unwanted, out)
		}
	}
}

func TestList_EmptyRendersEmptyState(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"emails": []map[string]any{},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No emails found.") {
		t.Fatalf("expected empty message, got %q", out)
	}
}

func TestList_EmptyPageRendersPaginationHint(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"emails":        []map[string]any{},
			"next_page_key": "cursor_2",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--state", "draft"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"No emails found on this page.",
		"More results available: gumroad emails list --state draft --page-key cursor_2",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty page output missing %q in %q", want, out)
		}
	}
}

func TestList_InvalidStateReturnsUsageError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called for invalid state")
	})

	cmd := testutil.Command(newListCmd())
	cmd.SetArgs([]string{"--state", "sent"})
	err := cmd.Execute()

	assertUsageError(t, err, "--state must be one of: published, scheduled, draft")
}

func TestList_AllFollowsPageKey(t *testing.T) {
	var queries []url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query())
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"emails": []map[string]any{
					emailPayload("email_1", "First", "draft"),
				},
				"next_page_key": "cursor_2",
			})
		case "cursor_2":
			testutil.JSON(t, w, map[string]any{
				"emails": []map[string]any{
					emailPayload("email_2", "Second", "published"),
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if len(queries) != 2 {
		t.Fatalf("got %d requests, want 2", len(queries))
	}
	if queries[1].Get("page_key") != "cursor_2" {
		t.Fatalf("second page_key = %q, want cursor_2", queries[1].Get("page_key"))
	}
	if !strings.Contains(out, "email_1\tFirst") || !strings.Contains(out, "email_2\tSecond") {
		t.Fatalf("paginated output missing rows: %q", out)
	}
}

func TestList_AllJSONFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			first := completeEmailPayload("email_1", "First", "draft")
			first["message"] = "<p>First body</p>"
			first["product_id"] = nil
			first["url"] = nil
			testutil.JSON(t, w, map[string]any{
				"emails":        []map[string]any{first},
				"next_page_key": "cursor_2",
				"next_page_url": "https://example.com/emails?page_key=cursor_2",
			})
		case "cursor_2":
			testutil.JSON(t, w, map[string]any{
				"emails": []map[string]any{
					completeEmailPayload("email_2", "Second", "published"),
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Emails []map[string]any `json:"emails"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Emails) != 2 {
		t.Fatalf("got %d emails, want 2", len(resp.Emails))
	}
	if resp.Emails[0]["message"] != "<p>First body</p>" {
		t.Fatalf("message = %v, want raw API message", resp.Emails[0]["message"])
	}
	if got, ok := resp.Emails[0]["product_id"]; !ok || got != nil {
		t.Fatalf("product_id = %#v, want preserved null", got)
	}
	if _, ok := resp.Emails[0]["audience_count"]; !ok {
		t.Fatalf("audience_count missing from raw email object: %#v", resp.Emails[0])
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_AllJQPreservesRawEmailFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		item := completeEmailPayload("email_1", "First", "draft")
		item["message"] = "<p>First body</p>"
		testutil.JSON(t, w, map[string]any{
			"emails": []map[string]any{item},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JQ(".emails[].message"))
	cmd.SetArgs([]string{"--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var got string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("jq output is not a JSON string: %v\n%s", err, out)
	}
	if got != "<p>First body</p>" {
		t.Fatalf("jq output = %q, want raw message", got)
	}
}

func TestList_JSONPassesRawResponse(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"emails": []map[string]any{
				emailPayload("email_123", "Draft note", "draft"),
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if _, ok := resp["emails"]; !ok {
		t.Fatalf("JSON response missing emails: %q", out)
	}
}

func TestView_RendersFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Launch", "State: published", "Audience: all", "Send emails: yes", "URL: https://example.com/emails/email_123", "Published at: 2026-06-17T10:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view output missing %q in %q", want, out)
		}
	}
}

func TestView_ScheduledShowsScheduledDate(t *testing.T) {
	scheduled := completeEmailPayload("email_sched", "Upcoming", "scheduled")
	scheduled["published_at"] = ""
	scheduled["scheduled_at"] = "2026-06-18T14:00:00Z"

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": scheduled})
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_sched"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Scheduled at: 2026-06-18T14:00:00Z") {
		t.Fatalf("scheduled view missing scheduled date in %q", out)
	}
	if strings.Contains(out, "Published at:") {
		t.Fatalf("scheduled view must not show a published date in %q", out)
	}
}

func TestView_PlainScheduledShowsScheduledDate(t *testing.T) {
	scheduled := completeEmailPayload("email_sched", "Upcoming", "scheduled")
	scheduled["published_at"] = ""
	scheduled["scheduled_at"] = "2026-06-18T14:00:00Z"

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": scheduled})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"email_sched"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.HasSuffix(out, "\t2026-06-18T14:00:00Z\n") {
		t.Fatalf("plain scheduled view missing scheduled date column in %q", out)
	}
}

func TestView_PlainOutputRendersPublishedFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": completeEmailPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "email_123\tLaunch\tpublished\tall\t\tyes\thttps://example.com/emails/email_123\t2026-06-17T10:00:00Z\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestView_DraftOutputRendersNoSendEmailsAndOmitsNullFields(t *testing.T) {
	draft := completeEmailPayload("email_draft", "Draft update", "draft")
	draft["audience_type"] = "follower"
	draft["published_at"] = "2026-06-17T10:00:00Z"
	draft["scheduled_at"] = nil
	draft["send_emails"] = false
	draft["url"] = nil
	draft["recipients_count"] = nil

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": draft})
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_draft"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Draft update", "State: draft", "Audience: followers", "Send emails: no"} {
		if !strings.Contains(out, want) {
			t.Fatalf("draft view output missing %q in %q", want, out)
		}
	}
	for _, unwanted := range []string{"URL:", "Published at:"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("draft view output contains %q in %q", unwanted, out)
		}
	}
}

func TestView_RendersProductTargetAndFriendlyAudience(t *testing.T) {
	product := completeEmailPayload("email_product", "Product update", "draft")
	product["audience_type"] = "product"
	product["product_id"] = "prod_123"

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": product})
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_product"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Product update", "Audience: product", "Product ID: prod_123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("product view output missing %q in %q", want, out)
		}
	}
}

func TestView_JSONPassesRawResponse(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if _, ok := resp["email"]; !ok {
		t.Fatalf("JSON response missing email: %q", out)
	}
}

func TestSend_YesPostsSendEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{"email": emailPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newSendCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/emails/email_123/send" {
		t.Fatalf("got %s %s, want POST /emails/email_123/send", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Sent email:") || !strings.Contains(out, "published") {
		t.Fatalf("unexpected send output: %q", out)
	}
}

func TestSend_YesPlainOutputPrintsEmailFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"email": completeEmailPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newSendCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "email_123\tLaunch\tpublished\n" {
		t.Fatalf("got %q, want plain sent email row", out)
	}
}

func TestSend_DeclinedConfirmationCancelsBeforeAPI(t *testing.T) {
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("API must not be called when confirmation is declined")
	})

	var stderr strings.Builder
	cmd := testutil.Command(newSendCmd(), testutil.Quiet(false), testutil.Stdin(declinedConfirmationInput(t)), testutil.Stderr(&stderr))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if called {
		t.Fatal("API was called after declined confirmation")
	}
	if !strings.Contains(out, "Cancelled: send email email_123.") {
		t.Fatalf("unexpected cancellation output: %q", out)
	}
	if !strings.Contains(stderr.String(), "Send email email_123 to its audience now?") {
		t.Fatalf("confirmation prompt missing from stderr: %q", stderr.String())
	}
}

func TestSend_NonInteractiveWithoutYesFailsBeforeAPI(t *testing.T) {
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("API must not be called without confirmation")
	})

	cmd := testutil.Command(newSendCmd(), testutil.NonInteractive(true))
	cmd.SetArgs([]string{"email_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes confirmation error, got %v", err)
	}
	if called {
		t.Fatal("API was called before confirmation")
	}
}

func TestDelete_YesDeletesEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{"message": "Deleted"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodDelete || gotPath != "/emails/email_123" {
		t.Fatalf("got %s %s, want DELETE /emails/email_123", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Email email_123 deleted.") {
		t.Fatalf("unexpected delete output: %q", out)
	}
}

func TestDelete_NoInputWithoutYesFailsBeforeAPI(t *testing.T) {
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("API must not be called without confirmation")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"email_123"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes confirmation error, got %v", err)
	}
	if called {
		t.Fatal("API was called before confirmation")
	}
}

func TestDelete_CancelledOutputUsesSharedFormat(t *testing.T) {
	opts := testutil.TestOptions(testutil.Quiet(false))
	var out strings.Builder
	opts.Stdout = &out

	if err := cmdutil.PrintCancelledAction(opts, "delete email email_123", "email_123"); err != nil {
		t.Fatalf("PrintCancelledAction failed: %v", err)
	}
	if !strings.Contains(out.String(), "Cancelled: delete email email_123.") {
		t.Fatalf("unexpected cancel output: %q", out.String())
	}
}
