package email

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

func emailInstallmentPayload(id, subject, state string) map[string]any {
	return map[string]any{
		"id":               id,
		"subject":          subject,
		"message":          "<p>Hello</p>",
		"audience_type":    "all",
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

func completeEmailInstallmentPayload(id, subject, state string) map[string]any {
	item := emailInstallmentPayload(id, subject, state)
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

func TestNewEmailCmd_HelpMentionsDraftPreviewWorkflow(t *testing.T) {
	cmd := NewEmailCmd()
	if !strings.Contains(cmd.Long, "created as drafts by default") {
		t.Fatalf("expected draft default in help, got %q", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "gumroad email preview <id>") {
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
		testutil.JSON(t, w, map[string]any{"installment": emailInstallmentPayload("email_123", r.PostForm.Get("subject"), "draft")})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--subject", "Launch", "--body", bodyPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/installments" {
		t.Fatalf("got %s %s, want POST /installments", gotMethod, gotPath)
	}
	if gotForm.Get("subject") != "Launch" {
		t.Fatalf("subject = %q, want Launch", gotForm.Get("subject"))
	}
	if gotForm.Get("body") != "<h1>Launch</h1>" {
		t.Fatalf("body = %q", gotForm.Get("body"))
	}
	if gotForm.Get("audience") != "all" {
		t.Fatalf("audience = %q, want all", gotForm.Get("audience"))
	}
	if _, ok := gotForm["publish"]; ok {
		t.Fatal("publish must be omitted for default draft creation")
	}
	if !strings.Contains(out, "Created email:") || !strings.Contains(out, "Launch") || !strings.Contains(out, "email_123") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCreate_DraftFalsePublishes(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Now</p>")
	var gotPublish string

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPublish = r.PostForm.Get("publish")
		testutil.JSON(t, w, map[string]any{"installment": emailInstallmentPayload("email_123", "Now", "published")})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--subject", "Now", "--body", bodyPath, "--draft=false"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPublish != "true" {
		t.Fatalf("publish = %q, want true", gotPublish)
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

func TestCreate_ProductAudienceSendsLinkID(t *testing.T) {
	bodyPath := writeEmailBody(t, "<p>Product update</p>")
	var gotLinkID string

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotLinkID = r.PostForm.Get("link_id")
		testutil.JSON(t, w, map[string]any{"installment": emailInstallmentPayload("email_123", "Product update", "draft")})
	})

	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--subject", "Product update", "--body", bodyPath, "--audience", "product", "--product", "prod_123"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotLinkID != "prod_123" {
		t.Fatalf("link_id = %q, want prod_123", gotLinkID)
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
	for _, want := range []string{"POST", "/installments", "subject: Preview params", "audience: followers", "body: <p>Preview params</p>"} {
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

	cmd := testutil.Command(newPreviewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/installments/email_123/preview" {
		t.Fatalf("got %s %s, want POST /installments/email_123/preview", gotMethod, gotPath)
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

	cmd := testutil.Command(newPreviewCmd(), testutil.Quiet(false))
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

	cmd := testutil.Command(newPreviewCmd(), testutil.PlainOutput())
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
			"installments": []map[string]any{
				emailInstallmentPayload("email_123", "Draft note", "draft"),
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
	scheduled := completeEmailInstallmentPayload("email_scheduled", "Scheduled note", "scheduled")
	scheduled["published_at"] = ""
	scheduled["scheduled_at"] = "2026-06-18T14:00:00Z"

	draft := completeEmailInstallmentPayload("email_draft", "Draft note", "draft")
	draft["published_at"] = ""
	draft["scheduled_at"] = ""

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"installments": []map[string]any{
				completeEmailInstallmentPayload("email_published", "Published note", "published"),
				scheduled,
				draft,
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"email_published\tPublished note\tpublished\tall\t2026-06-17T10:00:00Z\n",
		"email_scheduled\tScheduled note\tscheduled\tall\t2026-06-18T14:00:00Z\n",
		"email_draft\tDraft note\tdraft\tall\t\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain list output missing %q in %q", want, out)
		}
	}
}

func TestList_EmptyRendersEmptyState(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"installments": []map[string]any{},
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
			"installments":  []map[string]any{},
			"next_page_key": "cursor_2",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--state", "draft"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"No emails found on this page.",
		"More results available: gumroad email list --state draft --page-key cursor_2",
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
				"installments": []map[string]any{
					emailInstallmentPayload("email_1", "First", "draft"),
				},
				"next_page_key": "cursor_2",
			})
		case "cursor_2":
			testutil.JSON(t, w, map[string]any{
				"installments": []map[string]any{
					emailInstallmentPayload("email_2", "Second", "published"),
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
			testutil.JSON(t, w, map[string]any{
				"installments": []map[string]any{
					completeEmailInstallmentPayload("email_1", "First", "draft"),
				},
				"next_page_key": "cursor_2",
				"next_page_url": "https://example.com/installments?page_key=cursor_2",
			})
		case "cursor_2":
			testutil.JSON(t, w, map[string]any{
				"installments": []map[string]any{
					completeEmailInstallmentPayload("email_2", "Second", "published"),
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
		Installments []map[string]any `json:"installments"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Installments) != 2 {
		t.Fatalf("got %d installments, want 2", len(resp.Installments))
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_JSONPassesRawResponse(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"installments": []map[string]any{
				emailInstallmentPayload("email_123", "Draft note", "draft"),
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
	if _, ok := resp["installments"]; !ok {
		t.Fatalf("JSON response missing installments: %q", out)
	}
}

func TestView_RendersFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"installment": emailInstallmentPayload("email_123", "Launch", "published")})
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

func TestView_PlainOutputRendersPublishedFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"installment": completeEmailInstallmentPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "email_123\tLaunch\tpublished\tall\tyes\thttps://example.com/emails/email_123\t2026-06-17T10:00:00Z\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestView_DraftOutputRendersNoSendEmailsAndOmitsNullFields(t *testing.T) {
	draft := completeEmailInstallmentPayload("email_draft", "Draft update", "draft")
	draft["audience_type"] = "followers"
	draft["published_at"] = nil
	draft["scheduled_at"] = nil
	draft["send_emails"] = false
	draft["url"] = nil
	draft["recipients_count"] = nil

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"installment": draft})
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

func TestView_JSONPassesRawResponse(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"installment": emailInstallmentPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if _, ok := resp["installment"]; !ok {
		t.Fatalf("JSON response missing installment: %q", out)
	}
}

func TestSend_YesPostsSendEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{"installment": emailInstallmentPayload("email_123", "Launch", "published")})
	})

	cmd := testutil.Command(newSendCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"email_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/installments/email_123/send" {
		t.Fatalf("got %s %s, want POST /installments/email_123/send", gotMethod, gotPath)
	}
	if !strings.Contains(out, "Sent email:") || !strings.Contains(out, "published") {
		t.Fatalf("unexpected send output: %q", out)
	}
}

func TestSend_YesPlainOutputPrintsEmailFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"installment": completeEmailInstallmentPayload("email_123", "Launch", "published")})
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

	if gotMethod != http.MethodDelete || gotPath != "/installments/email_123" {
		t.Fatalf("got %s %s, want DELETE /installments/email_123", gotMethod, gotPath)
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
