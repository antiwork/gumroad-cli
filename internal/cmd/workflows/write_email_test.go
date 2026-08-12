package workflows

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func writeWorkflowEmailBody(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "email.html")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	return path
}

func assertWorkflowUsageError(t *testing.T, err error, want string) {
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

func savedEmailHandler(t *testing.T, gotMethod *string, gotPath *string, gotForm *map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if gotMethod != nil {
			*gotMethod = r.Method
		}
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if gotForm != nil {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm failed: %v", err)
			}
			form := map[string]string{}
			for key := range r.PostForm {
				form[key] = r.PostForm.Get(key)
			}
			*gotForm = form
		}
		testutil.JSON(t, w, map[string]any{"email": workflowEmailPayload("e9", "Week 4 check-in")})
	}
}

func TestParseDelayFlag(t *testing.T) {
	cases := []struct {
		in         string
		wantAmount int
		wantUnit   string
		wantErr    string
	}{
		{in: "4 weeks", wantAmount: 4, wantUnit: "week"},
		{in: "1 hour", wantAmount: 1, wantUnit: "hour"},
		{in: "0 days", wantAmount: 0, wantUnit: "day"},
		{in: "2 Months", wantAmount: 2, wantUnit: "month"},
		{in: "4weeks", wantErr: `expected "<amount> <unit>"`},
		{in: "week 4", wantErr: "amount must be a non-negative integer"},
		{in: "-1 week", wantErr: "amount must be a non-negative integer"},
		{in: "3 fortnights", wantErr: "unit must be one of"},
		{in: "", wantErr: `expected "<amount> <unit>"`},
	}
	for _, tc := range cases {
		amount, unit, err := parseDelayFlag(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("parseDelayFlag(%q) err = %v, want containing %q", tc.in, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDelayFlag(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if amount != tc.wantAmount || unit != tc.wantUnit {
			t.Errorf("parseDelayFlag(%q) = (%d, %q), want (%d, %q)", tc.in, amount, unit, tc.wantAmount, tc.wantUnit)
		}
	}
}

func TestAddEmail_PostsEndpointAndParams(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm map[string]string
	testutil.Setup(t, savedEmailHandler(t, &gotMethod, &gotPath, &gotForm))

	bodyPath := writeWorkflowEmailBody(t, "<p>See you in week four</p>")
	cmd := testutil.Command(newAddEmailCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"w1", "--subject", "Week 4 check-in", "--body", bodyPath, "--delay", "4 weeks"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPost || gotPath != "/workflows/w1/emails" {
		t.Fatalf("got %s %s, want POST /workflows/w1/emails", gotMethod, gotPath)
	}
	want := map[string]string{
		"subject":      "Week 4 check-in",
		"body":         "<p>See you in week four</p>",
		"delay_amount": "4",
		"delay_unit":   "week",
	}
	for key, wantValue := range want {
		if gotForm[key] != wantValue {
			t.Errorf("param %s = %q, want %q", key, gotForm[key], wantValue)
		}
	}
	if !strings.Contains(out, "Added email step:") || !strings.Contains(out, "Week 4 check-in") {
		t.Errorf("output missing confirmation: %q", out)
	}
}

func TestAddEmail_MissingFlagsRejectedBeforeRequest(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called when required flags are missing")
	})

	bodyPath := writeWorkflowEmailBody(t, "<p>Hi</p>")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "subject", args: []string{"w1", "--body", bodyPath, "--delay", "4 weeks"}, want: "--subject"},
		{name: "body", args: []string{"w1", "--subject", "Hi", "--delay", "4 weeks"}, want: "--body"},
		{name: "delay", args: []string{"w1", "--subject", "Hi", "--body", bodyPath}, want: "--delay"},
		{name: "bad delay", args: []string{"w1", "--subject", "Hi", "--body", bodyPath, "--delay", "4 fortnights"}, want: "unit must be one of"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := testutil.Command(newAddEmailCmd())
			cmd.SetArgs(tc.args)
			assertWorkflowUsageError(t, cmd.Execute(), tc.want)
		})
	}
}

func TestAddEmail_DryRunPrintsRequestWithoutCallingAPI(t *testing.T) {
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("API must not be called during dry-run")
	})

	bodyPath := writeWorkflowEmailBody(t, "<p>Preview</p>")
	cmd := testutil.Command(newAddEmailCmd(), testutil.DryRun(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"w1", "--subject", "Preview", "--body", bodyPath, "--delay", "2 days"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if called {
		t.Fatal("API was called during dry-run")
	}
	for _, want := range []string{"POST", "/workflows/w1/emails", "subject: Preview", "delay_amount: 2", "delay_unit: day"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q in %q", want, out)
		}
	}
}

func TestAddEmail_JSON(t *testing.T) {
	testutil.Setup(t, savedEmailHandler(t, nil, nil, nil))

	bodyPath := writeWorkflowEmailBody(t, "<p>Hi</p>")
	cmd := testutil.Command(newAddEmailCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"w1", "--subject", "Week 4 check-in", "--body", bodyPath, "--delay", "4 weeks"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, `"id"`) || !strings.Contains(out, "e9") {
		t.Errorf("JSON output missing email payload: %q", out)
	}
}

func TestUpdateEmail_PutsOnlyChangedFields(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm map[string]string
	testutil.Setup(t, savedEmailHandler(t, &gotMethod, &gotPath, &gotForm))

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"w1", "e9", "--subject", "New subject"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != "/workflows/w1/emails/e9" {
		t.Fatalf("got %s %s, want PUT /workflows/w1/emails/e9", gotMethod, gotPath)
	}
	if gotForm["subject"] != "New subject" {
		t.Errorf("subject = %q, want %q", gotForm["subject"], "New subject")
	}
	for _, absent := range []string{"body", "delay_amount", "delay_unit"} {
		if _, ok := gotForm[absent]; ok {
			t.Errorf("param %s must not be sent when its flag is unset (got %q)", absent, gotForm[absent])
		}
	}
	if !strings.Contains(out, "Updated email step:") {
		t.Errorf("output missing confirmation: %q", out)
	}
}

func TestUpdateEmail_DelaySendsBothDelayParams(t *testing.T) {
	var gotForm map[string]string
	testutil.Setup(t, savedEmailHandler(t, nil, nil, &gotForm))

	cmd := testutil.Command(newUpdateEmailCmd())
	cmd.SetArgs([]string{"w1", "e9", "--delay", "1 hour"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotForm["delay_amount"] != "1" || gotForm["delay_unit"] != "hour" {
		t.Errorf("delay params = %q/%q, want 1/hour", gotForm["delay_amount"], gotForm["delay_unit"])
	}
}

func TestUpdateEmail_BodyFromFile(t *testing.T) {
	var gotForm map[string]string
	testutil.Setup(t, savedEmailHandler(t, nil, nil, &gotForm))

	bodyPath := writeWorkflowEmailBody(t, "<p>Revised copy</p>")
	cmd := testutil.Command(newUpdateEmailCmd())
	cmd.SetArgs([]string{"w1", "e9", "--body", bodyPath})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotForm["body"] != "<p>Revised copy</p>" {
		t.Errorf("body = %q, want file contents", gotForm["body"])
	}
}

func TestUpdateEmail_NoFlagsRejectedBeforeRequest(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called when no update flags are set")
	})

	cmd := testutil.Command(newUpdateEmailCmd())
	cmd.SetArgs([]string{"w1", "e9"})
	assertWorkflowUsageError(t, cmd.Execute(), "at least one field to update")
}

func TestUpdateEmail_EmptySubjectRejectedBeforeRequest(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called with an empty subject")
	})

	cmd := testutil.Command(newUpdateEmailCmd())
	cmd.SetArgs([]string{"w1", "e9", "--subject", ""})
	assertWorkflowUsageError(t, cmd.Execute(), "--subject cannot be empty")
}

func TestUpdateEmail_MissingBodyFileRejectedBeforeRequest(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API must not be called when body file is missing")
	})

	missing := filepath.Join(t.TempDir(), "missing.html")
	cmd := testutil.Command(newUpdateEmailCmd())
	cmd.SetArgs([]string{"w1", "e9", "--body", missing})
	assertWorkflowUsageError(t, cmd.Execute(), "--body: cannot read")
}

func TestUpdateEmail_DryRunPrintsRequestWithoutCallingAPI(t *testing.T) {
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("API must not be called during dry-run")
	})

	cmd := testutil.Command(newUpdateEmailCmd(), testutil.DryRun(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"w1", "e9", "--subject", "Preview", "--delay", "3 weeks"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if called {
		t.Fatal("API was called during dry-run")
	}
	for _, want := range []string{"PUT", "/workflows/w1/emails/e9", "subject: Preview", "delay_amount: 3", "delay_unit: week"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q in %q", want, out)
		}
	}
}

func TestAddEmail_PlainOutput(t *testing.T) {
	testutil.Setup(t, savedEmailHandler(t, nil, nil, nil))

	bodyPath := writeWorkflowEmailBody(t, "<p>Hi</p>")
	cmd := testutil.Command(newAddEmailCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"w1", "--subject", "Week 4 check-in", "--body", bodyPath, "--delay", "4 weeks"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	line := strings.TrimSpace(out)
	if !strings.HasPrefix(line, "e9\t") || !strings.Contains(line, "Week 4 check-in") {
		t.Errorf("plain output missing id/subject columns: %q", out)
	}
}
