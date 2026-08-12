package workflows

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

const workflowEmailBodyFileMode = 0600

func writeWorkflowEmailBody(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "email.html")
	if err := os.WriteFile(path, []byte(body), workflowEmailBodyFileMode); err != nil {
		t.Fatalf("write body: %v", err)
	}
	return path
}

func workflowEmailWriteHandler(t *testing.T, inspect func(*http.Request)) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if inspect != nil {
			inspect(r)
		}
		testutil.JSON(t, w, map[string]any{"email": workflowEmailPayload("e1", r.PostForm.Get("subject"))})
	}
}

func requireWorkflowUsageError(t *testing.T, err error, message string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected usage error")
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("got %T, want *cmdutil.UsageError", err)
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("error %q does not contain %q", err, message)
	}
}

func TestAddEmail_PostsResolvedBodyAndDelay(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Keep going.</p>")
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, workflowEmailWriteHandler(t, func(r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotForm = r.PostForm
	}))

	var out bytes.Buffer
	cmd := testutil.Command(newAddEmailCmd(), testutil.Quiet(false), testutil.Stdout(&out), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Week four", "--body", bodyPath, "--delay", "4 weeks"})
	testutil.MustExecute(t, cmd)

	if gotMethod != http.MethodPost || gotPath != "/workflows/workflow_1/emails" {
		t.Fatalf("got %s %s, want POST /workflows/workflow_1/emails", gotMethod, gotPath)
	}
	if gotForm.Get("subject") != "Week four" || gotForm.Get("body") != "<p>Keep going.</p>" {
		t.Fatalf("unexpected content params: %#v", gotForm)
	}
	if gotForm.Get("delay_amount") != "4" || gotForm.Get("delay_unit") != "week" {
		t.Fatalf("unexpected delay params: %#v", gotForm)
	}
	if !strings.Contains(out.String(), "Added workflow email:") || !strings.Contains(out.String(), "Week four") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestAddEmail_ReadsBodyFromStdin(t *testing.T) {
	var gotBody string
	testutil.Setup(t, workflowEmailWriteHandler(t, func(r *http.Request) {
		gotBody = r.PostForm.Get("body")
	}))

	cmd := testutil.Command(newAddEmailCmd(), testutil.Stdin(strings.NewReader("<p>From stdin</p>")), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Welcome", "--body", "-", "--delay", "0 hours"})
	testutil.MustExecute(t, cmd)

	if gotBody != "<p>From stdin</p>" {
		t.Fatalf("body = %q, want stdin contents", gotBody)
	}
}

func TestAddEmail_DryRunShowsResolvedPayloadWithoutRequest(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Preview</p>")
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("dry run must not call the API")
	})

	var out bytes.Buffer
	cmd := testutil.Command(newAddEmailCmd(), testutil.DryRun(true), testutil.JSONOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Preview", "--body", bodyPath, "--delay", "1 DAY"})
	testutil.MustExecute(t, cmd)

	var payload struct {
		DryRun bool       `json:"dry_run"`
		Method string     `json:"method"`
		Path   string     `json:"path"`
		Params url.Values `json:"params"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse dry-run JSON: %v", err)
	}
	if !payload.DryRun || payload.Method != http.MethodPost || payload.Path != "/workflows/workflow_1/emails" {
		t.Fatalf("unexpected dry-run request: %#v", payload)
	}
	if payload.Params.Get("body") != "<p>Preview</p>" || payload.Params.Get("delay_unit") != "day" {
		t.Fatalf("unexpected dry-run params: %#v", payload.Params)
	}
}

func TestAddEmail_RequiresEveryFlag(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid input must not call the API")
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "subject", args: []string{"workflow_1", "--body", "email.html", "--delay", "1 day"}, want: "missing required flag: --subject"},
		{name: "body", args: []string{"workflow_1", "--subject", "Welcome", "--delay", "1 day"}, want: "missing required flag: --body"},
		{name: "delay", args: []string{"workflow_1", "--subject", "Welcome", "--body", "email.html"}, want: "missing required flag: --delay"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := testutil.Command(newAddEmailCmd())
			cmd.SetArgs(tc.args)
			requireWorkflowUsageError(t, cmd.Execute(), tc.want)
		})
	}
}

func TestParseWorkflowDelay_AcceptsSupportedUnits(t *testing.T) {
	tests := []struct {
		input      string
		wantAmount int64
		wantUnit   string
	}{
		{input: "0 hours", wantAmount: 0, wantUnit: "hour"},
		{input: "1 day", wantAmount: 1, wantUnit: "day"},
		{input: "2 WEEKS", wantAmount: 2, wantUnit: "week"},
		{input: "3 months", wantAmount: 3, wantUnit: "month"},
		{input: "816 months", wantAmount: 816, wantUnit: "month"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			delay, err := parseWorkflowDelay(tc.input)
			if err != nil {
				t.Fatalf("parse delay: %v", err)
			}
			if delay.Amount != tc.wantAmount || delay.Unit != tc.wantUnit {
				t.Fatalf("delay = %#v, want %d %s", delay, tc.wantAmount, tc.wantUnit)
			}
		})
	}
}

func TestParseWorkflowDelay_RejectsInvalidValues(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: "must use"},
		{input: "4", want: "must use"},
		{input: "four weeks", want: "non-negative integer"},
		{input: "-1 day", want: "non-negative integer"},
		{input: "1.5 days", want: "non-negative integer"},
		{input: "1 year", want: "unit must be one of"},
		{input: "817 months", want: "must not exceed"},
		{input: "596524 hours", want: "must not exceed"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			_, err := parseWorkflowDelay(tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want text %q", err, tc.want)
			}
		})
	}
}

func TestUpdateEmail_SendsOnlyTheBody(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Updated body</p>")
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, workflowEmailWriteHandler(t, func(r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotForm = r.PostForm
	}))

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"workflow_1", "email_1", "--body", bodyPath})
	testutil.MustExecute(t, cmd)

	if gotMethod != http.MethodPut || gotPath != "/workflows/workflow_1/emails/email_1" {
		t.Fatalf("got %s %s, want PUT /workflows/workflow_1/emails/email_1", gotMethod, gotPath)
	}
	if len(gotForm) != 1 || gotForm.Get("body") != "<p>Updated body</p>" {
		t.Fatalf("sparse update params = %#v", gotForm)
	}
}

func TestUpdateEmail_SendsSubjectAndDelay(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, workflowEmailWriteHandler(t, func(r *http.Request) {
		gotForm = r.PostForm
	}))

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "email_1", "--subject", "New subject", "--delay", "2 days"})
	testutil.MustExecute(t, cmd)

	if len(gotForm) != 3 || gotForm.Get("subject") != "New subject" || gotForm.Get("delay_amount") != "2" || gotForm.Get("delay_unit") != "day" {
		t.Fatalf("update params = %#v", gotForm)
	}
}

func TestUpdateEmail_RequiresAChangedField(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("empty update must not call the API")
	})

	cmd := testutil.Command(newUpdateEmailCmd())
	cmd.SetArgs([]string{"workflow_1", "email_1"})
	requireWorkflowUsageError(t, cmd.Execute(), "at least one field to update must be provided")
}

func TestEmailWrites_RequireConfirmationForSchedulingChanges(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Body</p>")
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("an unconfirmed write must not call the API")
	})

	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{
			name: "add",
			cmd:  newAddEmailCmd(),
			args: []string{"workflow_1", "--subject", "Welcome", "--body", bodyPath, "--delay", "1 day"},
		},
		{
			name: "delay update",
			cmd:  newUpdateEmailCmd(),
			args: []string{"workflow_1", "email_1", "--delay", "2 days"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := testutil.Command(tc.cmd, testutil.NoInput(true))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if !strings.Contains(fmt.Sprint(err), "confirmation required") {
				t.Fatalf("error = %v, want confirmation requirement", err)
			}
		})
	}
}

func TestUpdateEmail_RejectsEmptySubjectAndBodyPath(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid update must not call the API")
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "subject", args: []string{"workflow_1", "email_1", "--subject", " "}, want: "--subject must be a non-empty string"},
		{name: "body", args: []string{"workflow_1", "email_1", "--body", ""}, want: "--body must be a file path"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := testutil.Command(newUpdateEmailCmd())
			cmd.SetArgs(tc.args)
			requireWorkflowUsageError(t, cmd.Execute(), tc.want)
		})
	}
}

func TestUpdateEmail_JSONPreservesAPIResponse(t *testing.T) {
	testutil.Setup(t, workflowEmailWriteHandler(t, nil))

	var out bytes.Buffer
	cmd := testutil.Command(newUpdateEmailCmd(), testutil.JSONOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{"workflow_1", "email_1", "--subject", "Updated"})
	testutil.MustExecute(t, cmd)

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse JSON response: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("response = %#v", payload)
	}
	email, ok := payload["email"].(map[string]any)
	if !ok || email["id"] != "e1" || email["subject"] != "Updated" {
		t.Fatalf("email response = %#v", payload["email"])
	}
}

func TestUpdateEmail_PlainOutput(t *testing.T) {
	testutil.Setup(t, workflowEmailWriteHandler(t, nil))

	var out bytes.Buffer
	cmd := testutil.Command(newUpdateEmailCmd(), testutil.PlainOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{"workflow_1", "email_1", "--subject", "Updated"})
	testutil.MustExecute(t, cmd)

	if out.String() != "e1\tUpdated\t4 weeks\tpublished\n" {
		t.Fatalf("plain output = %q", out.String())
	}
}

func TestEmailWrite_InvalidJQFailsBeforeRequest(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Body</p>")
	reached := false
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		reached = true
	})

	cmd := testutil.Command(newAddEmailCmd(), testutil.JQ(".["), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Welcome", "--body", bodyPath, "--delay", "1 day"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid jq expression") {
		t.Fatalf("error = %v, want invalid jq expression", err)
	}
	if reached {
		t.Fatal("invalid jq expression caused a write request")
	}
}

func TestEmailWrite_JQRuntimeErrorPreservesCompletedWrite(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Body</p>")
	testutil.Setup(t, workflowEmailWriteHandler(t, nil))

	var out bytes.Buffer
	cmd := testutil.Command(newAddEmailCmd(), testutil.JQ(".email.id | tonumber"), testutil.Yes(true), testutil.Stdout(&out))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"workflow_1", "--subject", "Welcome", "--body", bodyPath, "--delay", "1 day"})
	err := cmd.Execute()
	var completed *CompletedEmailWriteOutputError
	if !errors.As(err, &completed) {
		t.Fatalf("error = %v, want CompletedEmailWriteOutputError", err)
	}
	if completed.WorkflowID != "workflow_1" || completed.EmailID != "e1" || completed.Action != EmailWriteActionAdd {
		t.Fatalf("completed write = %#v", completed)
	}
	if !strings.Contains(completed.RecoveryHint(), "Do not retry") {
		t.Fatalf("hint = %q", completed.RecoveryHint())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout contains partial jq output: %q", out.String())
	}
}

func TestEmailWrite_InvalidSuccessResponseHasUnknownState(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Body</p>")
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RawJSON(t, w, `{"success":true,"email":{}}`)
	})

	cmd := testutil.Command(newAddEmailCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Welcome", "--body", bodyPath, "--delay", "1 day"})
	err := cmd.Execute()
	var unknown *UnknownEmailWriteError
	if !errors.As(err, &unknown) || unknown.WorkflowID != "workflow_1" || unknown.Action != EmailWriteActionAdd {
		t.Fatalf("error = %v, want unknown add state", err)
	}
}

func TestEmailWrite_TransientFailuresHaveUnknownState(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Body</p>")
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream failure", http.StatusBadGateway)
	})

	cmd := testutil.Command(newAddEmailCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Welcome", "--body", bodyPath, "--delay", "1 day"})
	err := cmd.Execute()
	var unknown *UnknownEmailWriteError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want UnknownEmailWriteError", err)
	}
	if unknown.Action != EmailWriteActionAdd || unknown.WorkflowID != "workflow_1" || unknown.EmailID != "" {
		t.Fatalf("unknown write = %#v", unknown)
	}
	if !strings.Contains(unknown.RecoveryHint(), "Do not retry automatically") {
		t.Fatalf("hint = %q", unknown.RecoveryHint())
	}
}

func TestEmailWrite_DefinitiveRejectionStaysAPIError(t *testing.T) {
	bodyPath := writeWorkflowEmailBody(t, "<p>Body</p>")
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		testutil.RawJSON(t, w, `{"success":false,"message":"invalid email"}`)
	})

	cmd := testutil.Command(newAddEmailCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"workflow_1", "--subject", "Welcome", "--body", bodyPath, "--delay", "1 day"})
	err := cmd.Execute()
	var unknown *UnknownEmailWriteError
	if errors.As(err, &unknown) {
		t.Fatalf("definitive rejection became unknown: %v", err)
	}
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want 422 APIError", err)
	}
}

func TestNewWorkflowsCmd_HelpWarnsAboutScheduling(t *testing.T) {
	cmd := NewWorkflowsCmd()
	if !strings.Contains(cmd.Long, "published workflow can schedule recipients") {
		t.Fatalf("help does not warn about scheduling: %q", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "delay change can reschedule recipients") {
		t.Fatalf("help does not warn about rescheduling: %q", cmd.Long)
	}
}
