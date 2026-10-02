package products

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func createProductHandler(t *testing.T, product map[string]any, warning string, gotForm *url.Values) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if gotForm != nil {
			_ = r.ParseForm()
			*gotForm = r.PostForm
		}
		body := map[string]any{"product": product}
		if warning != "" {
			body["warning"] = warning
		}
		testutil.JSON(t, w, body)
	}
}

func TestCreate_OmitsDraftParamByDefault(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "published": true}, "", &gotForm))

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10.00"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if _, sent := gotForm["draft"]; sent {
		t.Fatalf("draft should be omitted without --draft, got %q", gotForm.Get("draft"))
	}
}

func TestCreate_DraftFlagSendsDraftParam(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "published": false}, "", &gotForm))

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--draft"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := gotForm.Get("draft"); got != "true" {
		t.Fatalf("draft = %q, want true", got)
	}
}

func TestCreate_PublishedProductOutputPointsAtUnpublish(t *testing.T) {
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "formatted_price": "$10", "published": true}, "", nil))

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10.00"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Created and published product:") {
		t.Errorf("expected published confirmation, got: %q", out)
	}
	if !strings.Contains(out, "Unpublish with: gumroad products unpublish p1") {
		t.Errorf("expected unpublish tip, got: %q", out)
	}
}

func TestCreate_DraftProductOutputPointsAtPublish(t *testing.T) {
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "formatted_price": "$10", "published": false}, "", nil))

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10.00", "--draft"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Created draft product:") {
		t.Errorf("expected draft confirmation, got: %q", out)
	}
	if !strings.Contains(out, "Publish with: gumroad products publish p1") {
		t.Errorf("expected publish tip, got: %q", out)
	}
}

func TestCreate_BlockedPublishPrintsWarning(t *testing.T) {
	warning := "Saved as a draft: You have to confirm your email address before you can do that."
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "published": false}, warning, nil))

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, warning) {
		t.Errorf("expected the server warning in output, got: %q", out)
	}
	if !strings.Contains(out, "Created draft product:") {
		t.Errorf("expected draft confirmation, got: %q", out)
	}
	if !strings.Contains(out, "Publish with: gumroad products publish p1") {
		t.Errorf("expected publish tip, got: %q", out)
	}
}

func TestCreate_WarningControlCharactersAreEscaped(t *testing.T) {
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "published": false},
		"Saved as a draft: bad\x1b[31mvalue", nil))

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, `bad\x1b[31mvalue`) {
		t.Errorf("expected escaped warning, got: %q", out)
	}
	if strings.Contains(out, "\x1b[31m") {
		t.Errorf("raw escape sequence reached stdout: %q", out)
	}
}

func TestCreate_BlockedPublishPlainKeepsColumnsAndWarnsOnStderr(t *testing.T) {
	warning := "Saved as a draft: You have to confirm your email address before you can do that."
	testutil.Setup(t, createProductHandler(t,
		map[string]any{"id": "p1", "name": "Art Pack", "formatted_price": "$10", "published": false}, warning, nil))

	cmd := testutil.Command(newCreateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10.00"})
	stdout, stderr := testutil.CaptureOutput(func() { testutil.MustExecute(t, cmd) })

	if got := strings.TrimRight(stdout, "\n"); got != "p1	Art Pack	$10" {
		t.Errorf("plain stdout = %q, want the three columns unchanged", got)
	}
	if !strings.Contains(stderr, warning) {
		t.Errorf("expected the warning on stderr, got: %q", stderr)
	}
}

func TestCreate_DryRunShowsDraftFlag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry run should not reach the API")
	})

	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--name", "Art Pack", "--draft"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var payload struct {
		DryRun bool                `json:"dry_run"`
		Params map[string][]string `json:"params"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output is not valid JSON: %v\n%s", err, out)
	}
	if !payload.DryRun {
		t.Errorf("dry_run = false, want true")
	}
	if got := payload.Params["draft"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("draft param = %v, want [true]", got)
	}
}

func TestCreate_DryRunOmitsDraftWithoutFlag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry run should not reach the API")
	})

	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--name", "Art Pack"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Contains(out, "draft") {
		t.Errorf("dry-run body should not mention draft without --draft, got: %q", out)
	}
}
