package workflows

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

const (
	workflowTestEmailsCount = 2
	workflowTestSentCount   = 120
	workflowTestOpenCount   = 48
	workflowTestOpenRate    = 40.0
	workflowTestClickCount  = 6
	workflowTestClickRate   = 5.0
	workflowTestDelayWeeks  = 4
)

func workflowPayload(id, name string) map[string]any {
	return map[string]any{
		"id":                     id,
		"name":                   name,
		"audience_type":          "seller",
		"trigger":                nil,
		"product_id":             "p1",
		"variant_id":             nil,
		"state":                  "published",
		"published_at":           "2026-06-17T10:00:00Z",
		"first_published_at":     "2026-06-17T10:00:00Z",
		"send_to_past_customers": false,
		"emails_count":           workflowTestEmailsCount,
		"created_at":             "2026-06-17T09:00:00Z",
		"updated_at":             "2026-06-17T09:30:00Z",
	}
}

func workflowEmailPayload(id, subject string) map[string]any {
	return map[string]any{
		"id":            id,
		"subject":       subject,
		"message":       "<p>Hello</p>",
		"audience_type": "seller",
		"product_id":    "p1",
		"state":         "published",
		"published_at":  "2026-06-17T10:00:00Z",
		"send_emails":   true,
		"delay":         map[string]any{"amount": workflowTestDelayWeeks, "unit": "week"},
		"sent_count":    workflowTestSentCount,
		"open_count":    workflowTestOpenCount,
		"open_rate":     workflowTestOpenRate,
		"click_count":   workflowTestClickCount,
		"click_rate":    workflowTestClickRate,
		"created_at":    "2026-06-17T09:00:00Z",
		"updated_at":    "2026-06-17T09:30:00Z",
	}
}

func workflowListHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"workflows": []map[string]any{
				workflowPayload("w1", "Onboarding"),
				workflowPayload("w2", "Masterminds"),
			},
		})
	}
}

func workflowViewHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload := workflowPayload("w1", "Onboarding")
		payload["emails"] = []map[string]any{
			workflowEmailPayload("e1", "Welcome"),
			workflowEmailPayload("e2", "Week four check-in"),
		}
		testutil.JSON(t, w, map[string]any{"workflow": payload})
	}
}

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		workflowListHandler(t)(w, r)
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotPath != "/workflows" {
		t.Errorf("got path %q", gotPath)
	}
	if !strings.Contains(out, "Onboarding") || !strings.Contains(out, "Masterminds") {
		t.Errorf("output missing workflow names: %q", out)
	}
}

func TestList_TableShowsAudienceAndEmailsCount(t *testing.T) {
	testutil.Setup(t, workflowListHandler(t))

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "customers") {
		t.Errorf("output missing mapped audience label: %q", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("output missing emails count: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, workflowListHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	items := resp["workflows"].([]any)
	if len(items) != 2 {
		t.Errorf("got %d workflows, want 2", len(items))
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, workflowListHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d plain rows, want 2: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "w1\t") {
		t.Errorf("first row missing id column: %q", lines[0])
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"workflows": []map[string]any{}})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "No workflows found.") {
		t.Errorf("output missing empty message: %q", out)
	}
}

func TestView_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		workflowViewHandler(t)(w, r)
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"w1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if gotPath != "/workflows/w1" {
		t.Errorf("got path %q", gotPath)
	}
}

func TestView_TableShowsStepStats(t *testing.T) {
	testutil.Setup(t, workflowViewHandler(t))

	cmd := newViewCmd()
	cmd.SetArgs([]string{"w1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	for _, want := range []string{"Onboarding", "Welcome", "4 weeks", "120", "48", "40.0%", "6", "5.0%"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestView_NullRatesRenderAsEmptyFields(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		email := workflowEmailPayload("e1", "Unsent step")
		email["state"] = "draft"
		email["sent_count"] = 0
		email["open_count"] = 0
		email["open_rate"] = nil
		email["click_count"] = 0
		email["click_rate"] = nil
		payload := workflowPayload("w1", "Onboarding")
		payload["emails"] = []map[string]any{email}
		testutil.JSON(t, w, map[string]any{"workflow": payload})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"w1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	row := strings.TrimRight(out, "\n")
	columns := strings.Split(row, "\t")
	if len(columns) != 9 {
		t.Fatalf("got %d plain columns, want 9: %q", len(columns), row)
	}
	if columns[6] != "" || columns[8] != "" {
		t.Errorf("null rates not rendered as empty fields: %q", row)
	}
}

func TestView_PlainPrintsOneRowPerEmail(t *testing.T) {
	testutil.Setup(t, workflowViewHandler(t))

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"w1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d plain rows, want 2: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "e1\t") || !strings.HasPrefix(lines[1], "e2\t") {
		t.Errorf("plain rows missing email ids: %q", out)
	}
}

func TestView_NoEmails(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		payload := workflowPayload("w1", "Onboarding")
		payload["emails"] = []map[string]any{}
		testutil.JSON(t, w, map[string]any{"workflow": payload})
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"w1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "No emails in this workflow.") {
		t.Errorf("output missing empty emails message: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, workflowViewHandler(t))

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"w1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	workflow := resp["workflow"].(map[string]any)
	emails := workflow["emails"].([]any)
	if len(emails) != 2 {
		t.Errorf("got %d emails, want 2", len(emails))
	}
}

func TestView_MemberCancellationTriggerShown(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		payload := workflowPayload("w1", "Winback")
		payload["trigger"] = "member_cancellation"
		payload["emails"] = []map[string]any{workflowEmailPayload("e1", "Come back")}
		testutil.JSON(t, w, map[string]any{"workflow": payload})
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"w1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Trigger: member_cancellation") {
		t.Errorf("output missing trigger line: %q", out)
	}
}

func TestWorkflowDelayLabel_SingularUnit(t *testing.T) {
	label := workflowDelayLabel(workflowDelay{Amount: 1, Unit: "hour"})
	if label != "1 hour" {
		t.Errorf("got %q, want %q", label, "1 hour")
	}
}

func TestNewWorkflowsCmd_RegistersSubcommands(t *testing.T) {
	cmd := NewWorkflowsCmd()
	var names []string
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "list") || !strings.Contains(joined, "view") {
		t.Errorf("missing subcommands, got: %v", names)
	}
}
