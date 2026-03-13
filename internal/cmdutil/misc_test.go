package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type fakeSpinner struct {
	events *[]string
}

func (s *fakeSpinner) Start() {
	*s.events = append(*s.events, "start")
}

func (s *fakeSpinner) Stop() {
	*s.events = append(*s.events, "stop")
}

func installFakeSpinner(events *[]string) func() {
	previousSpinner := newSpinner
	newSpinner = func(string, io.Writer) spinner {
		return &fakeSpinner{events: events}
	}
	return func() {
		newSpinner = previousSpinner
	}
}

func setColorEnabledForTest(t *testing.T, enabled bool) {
	t.Helper()
	output.SetColorEnabledForTesting(enabled)
	t.Cleanup(output.ResetColorEnabledForTesting)
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Version != "dev" {
		t.Fatalf("got version %q, want dev", opts.Version)
	}
	if opts.Context == nil {
		t.Fatal("expected default context to be set")
	}
}

func TestWithOptionsRoundTrip(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	ctx := context.WithValue(context.Background(), contextKey("trace"), "abc")
	opts := DefaultOptions()
	opts.Version = "1.2.3"
	opts.Context = ctx
	cmd.SetContext(WithOptions(ctx, opts))

	got := OptionsFrom(cmd)
	if got.Version != "1.2.3" {
		t.Fatalf("got version %q, want 1.2.3", got.Version)
	}
	if got.Context.Value(contextKey("trace")) != "abc" {
		t.Fatalf("expected stored context value, got %v", got.Context.Value(contextKey("trace")))
	}
}

func TestOptionsFromFallback(t *testing.T) {
	got := OptionsFrom(&cobra.Command{Use: "demo"})
	if got.Version != "dev" {
		t.Fatalf("got version %q, want dev", got.Version)
	}
	if got.Context == nil {
		t.Fatal("expected default context")
	}
}

func TestUsesJSONOutput(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want bool
	}{
		{name: "default", opts: DefaultOptions(), want: false},
		{name: "json", opts: Options{JSONOutput: true}, want: true},
		{name: "jq", opts: Options{JQExpr: ".user.email"}, want: true},
	}

	for _, tc := range cases {
		if got := tc.opts.UsesJSONOutput(); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDebugEnabled(t *testing.T) {
	opts := DefaultOptions()
	if opts.DebugEnabled() {
		t.Fatal("debug should be disabled by default")
	}

	opts.Debug = true
	if !opts.DebugEnabled() {
		t.Fatal("debug flag should enable debug mode")
	}

	t.Setenv("GUMROAD_DEBUG", "1")
	opts.Debug = false
	if !opts.DebugEnabled() {
		t.Fatal("GUMROAD_DEBUG=1 should enable debug mode")
	}
}

func TestConfirmAction_DryRunSkipsPrompt(t *testing.T) {
	opts := DefaultOptions()
	opts.DryRun = true

	ok, err := ConfirmAction(opts, "Delete product prod_123?")
	if err != nil {
		t.Fatalf("ConfirmAction returned error: %v", err)
	}
	if !ok {
		t.Fatal("dry-run should auto-confirm actions")
	}
}

func TestCloneValuesCopiesNestedSlices(t *testing.T) {
	original := map[string][]string{
		"page": {"1"},
		"tag":  {"a", "b"},
	}

	cloned := CloneValues(original)
	cloned["page"][0] = "2"
	cloned["tag"] = append(cloned["tag"], "c")

	if original["page"][0] != "1" {
		t.Fatalf("original page mutated: %v", original["page"])
	}
	if len(original["tag"]) != 2 {
		t.Fatalf("original tag slice mutated: %v", original["tag"])
	}
}

func TestJoinPathEscapesSegments(t *testing.T) {
	got := JoinPath("products", "prod/1", "custom_fields", "name with space")
	want := "/products/prod%2F1/custom_fields/name%20with%20space"
	if got != want {
		t.Fatalf("got path %q, want %q", got, want)
	}
}

func TestPrintInfoAndSuccess(t *testing.T) {
	setColorEnabledForTest(t, false)
	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out
	if err := PrintInfo(opts, "hello"); err != nil {
		t.Fatalf("PrintInfo failed: %v", err)
	}
	if err := PrintSuccess(opts, "done"); err != nil {
		t.Fatalf("PrintSuccess failed: %v", err)
	}

	for _, want := range []string{"hello", "done"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}

	opts.Quiet = true
	out.Reset()
	if err := PrintInfo(opts, "quiet"); err != nil {
		t.Fatalf("PrintInfo failed: %v", err)
	}
	if err := PrintSuccess(opts, "quiet"); err != nil {
		t.Fatalf("PrintSuccess failed: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("expected no output in quiet mode, got %q", out.String())
	}
}

func TestUsageHelpers(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "demo <id>",
		Short:   "demo",
		Example: "  demo 123",
		Run:     func(*cobra.Command, []string) {},
	}
	cmd.Flags().Bool("name", false, "name")

	err := MissingFlagError(cmd, "--name")
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --name") {
		t.Fatalf("unexpected missing flag error: %v", err)
	}
	if !strings.Contains(err.Error(), "demo <id>") || !strings.Contains(err.Error(), "demo 123") {
		t.Fatalf("usage output missing expected help text: %v", err)
	}

	err = RequireAnyFlagChanged(cmd, "name")
	if err == nil || !strings.Contains(err.Error(), "at least one field to update") {
		t.Fatalf("unexpected require-any error: %v", err)
	}

	if err := cmd.Flags().Set("name", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireAnyFlagChanged(cmd, "name"); err != nil {
		t.Fatalf("expected changed flag to satisfy validation, got %v", err)
	}
}

func TestExactArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "demo <id>"}
	cmd.Example = "  demo 123"

	if err := ExactArgs(1)(cmd, []string{"123"}); err != nil {
		t.Fatalf("unexpected error for valid args: %v", err)
	}

	if err := ExactArgs(1)(cmd, nil); err == nil || !strings.Contains(err.Error(), "missing required argument: <id>") {
		t.Fatalf("unexpected missing arg error: %v", err)
	}

	if err := ExactArgs(1)(cmd, []string{"123", "extra"}); err == nil || !strings.Contains(err.Error(), "unexpected argument: extra") {
		t.Fatalf("unexpected extra arg error: %v", err)
	}
}

func TestPropagateExamplesFiltersToChildren(t *testing.T) {
	root := &cobra.Command{Use: "gumroad", Example: "  gumroad products list\n  gumroad products view <id>\n  gumroad sales list"}
	products := &cobra.Command{Use: "products", Run: func(*cobra.Command, []string) {}}
	view := &cobra.Command{Use: "view <id>", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(products)
	products.AddCommand(view)

	PropagateExamples(root)

	if !strings.Contains(products.Example, "gumroad products list") {
		t.Fatalf("products example missing inherited products example: %q", products.Example)
	}
	if strings.Contains(products.Example, "gumroad sales list") {
		t.Fatalf("products example should not include sibling example: %q", products.Example)
	}
	if !strings.Contains(view.Example, "gumroad products view <id>") {
		t.Fatalf("view example missing filtered leaf example: %q", view.Example)
	}
}

func TestPropagateExamplesGeneratesLeafFallback(t *testing.T) {
	root := &cobra.Command{Use: "gumroad", Example: "  gumroad custom-fields list --product <id>\n  gumroad custom-fields create --product <id> --name \"Company\" --required"}
	customFields := &cobra.Command{Use: "custom-fields", Run: func(*cobra.Command, []string) {}}
	update := &cobra.Command{Use: "update", Run: func(*cobra.Command, []string) {}}
	update.Flags().String("product", "", "Product ID (required)")
	update.Flags().String("name", "", "Field name (required)")
	root.AddCommand(customFields)
	customFields.AddCommand(update)

	PropagateExamples(root)

	if strings.Contains(update.Example, "gumroad custom-fields list") || strings.Contains(update.Example, "gumroad custom-fields create") {
		t.Fatalf("update example should not inherit unrelated ancestor examples: %q", update.Example)
	}
	for _, want := range []string{"gumroad custom-fields update", "--name <value>", "--product <value>"} {
		if !strings.Contains(update.Example, want) {
			t.Fatalf("update example missing %q in %q", want, update.Example)
		}
	}
	if strings.Contains(update.Example, "--required") {
		t.Fatalf("update example should not include optional flags: %q", update.Example)
	}
}

func TestRunCommandWithTokenUnsupportedMethod(t *testing.T) {
	err := RunWithToken(DefaultOptions(), "tok", "", func(client *api.Client) (json.RawMessage, error) {
		return runClientRequest(client, "PATCH", "/user", nil)
	}, func(json.RawMessage) error {
		t.Fatal("render should not run for unsupported method")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported HTTP method: PATCH") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRequestWithSuccess(t *testing.T) {
	setColorEnabledForTest(t, false)
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeSuccessJSON(t, w, map[string]any{})
	})

	opts := DefaultOptions()
	opts.Quiet = false
	opts.Version = "test"
	var out bytes.Buffer
	opts.Stdout = &out
	if err := RunRequestWithSuccess(opts, "Updating...", "PUT", "/licenses/enable", nil, "updated"); err != nil {
		t.Fatalf("RunRequestWithSuccess failed: %v", err)
	}
	if !strings.Contains(out.String(), "updated") {
		t.Fatalf("expected success output, got %q", out.String())
	}
}

func TestRunRequest_DryRunSkipsAPIAndAuth(t *testing.T) {
	opts := DefaultOptions()
	opts.DryRun = true
	opts.Version = "test"
	var out bytes.Buffer
	opts.Stdout = &out

	err := RunRequest(opts, "Refunding...", "PUT", "/sales/sale_123/refund", url.Values{
		"amount_cents": []string{"500"},
	}, func(json.RawMessage) error {
		t.Fatal("render should not run in dry-run mode")
		return nil
	})
	if err != nil {
		t.Fatalf("RunRequest failed: %v", err)
	}

	text := out.String()
	for _, want := range []string{"Dry run", "PUT /sales/sale_123/refund", "amount_cents: 500"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestRunRequest_DryRunJSONOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.DryRun = true
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := RunRequest(opts, "Updating...", "DELETE", "/products/prod_123", nil, func(json.RawMessage) error {
		t.Fatal("render should not run in dry-run mode")
		return nil
	})
	if err != nil {
		t.Fatalf("RunRequest failed: %v", err)
	}

	var resp struct {
		DryRun bool   `json:"dry_run"`
		Method string `json:"method"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("dry-run JSON output is invalid: %v\n%s", err, out.String())
	}
	if !resp.DryRun || resp.Method != "DELETE" || resp.Path != "/products/prod_123" {
		t.Fatalf("unexpected dry-run payload: %+v", resp)
	}
}

func TestRunRequestUsesCommandOptions(t *testing.T) {
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeSuccessJSON(t, w, map[string]any{"user": map[string]any{"email": "demo@example.com"}})
	})

	cmd := &cobra.Command{Use: "demo"}
	opts := DefaultOptions()
	opts.Context = context.Background()
	opts.JSONOutput = true
	opts.Version = "test"
	var out bytes.Buffer
	opts.Stdout = &out
	cmd.SetContext(WithOptions(opts.Context, opts))

	err := RunRequest(OptionsFrom(cmd), "Reading...", "GET", "/user", nil, func(json.RawMessage) error {
		t.Fatal("render should not run in JSON mode")
		return nil
	})
	if err != nil {
		t.Fatalf("RunRequest failed: %v", err)
	}
	if !strings.Contains(out.String(), "demo@example.com") {
		t.Fatalf("expected JSON output, got %q", out.String())
	}
}

func TestRun_StopsSpinnerBeforeRender(t *testing.T) {
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeSuccessJSON(t, w, map[string]any{"user": map[string]any{"email": "demo@example.com"}})
	})

	events := []string{}
	defer installFakeSpinner(&events)()

	opts := DefaultOptions()
	opts.Version = "test"
	opts.Quiet = false

	err := Run(opts, "Reading...", func(client *api.Client) (json.RawMessage, error) {
		return client.Get("/user", nil)
	}, func(json.RawMessage) error {
		events = append(events, "render")
		return nil
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	want := []string{"start", "stop", "render"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("got events %v, want %v", events, want)
	}
}

func TestRunWithTokenData_StopsSpinnerOnPanic(t *testing.T) {
	events := []string{}
	defer installFakeSpinner(&events)()

	opts := DefaultOptions()
	opts.Version = "test"
	opts.Quiet = false

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic")
		}

		want := []string{"start", "stop"}
		if !reflect.DeepEqual(events, want) {
			t.Fatalf("got events %v, want %v", events, want)
		}
	}()

	_, _ = runWithTokenData(opts, "test-token", "Reading...", func(*api.Client) (json.RawMessage, error) {
		panic("boom")
	})
}

func TestRunWithTokenData_DebugSkipsSpinner(t *testing.T) {
	events := []string{}
	defer installFakeSpinner(&events)()

	opts := DefaultOptions()
	opts.Version = "test"
	opts.Debug = true

	data, err := runWithTokenData(opts, "test-token", "Reading...", func(*api.Client) (json.RawMessage, error) {
		return json.RawMessage(`{"success":true}`), nil
	})
	if err != nil {
		t.Fatalf("runWithTokenData failed: %v", err)
	}
	if string(data) != `{"success":true}` {
		t.Fatalf("unexpected data: %s", data)
	}
	if len(events) != 0 {
		t.Fatalf("debug mode should skip spinner, got events %v", events)
	}
}

func setupAuthedAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	cfgDir := t.TempDir()
	configDir := filepath.Join(cfgDir, "gumroad")
	configPath := filepath.Join(configDir, "config.json")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"access_token":"test-token"}`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	srv := httptest.NewServer(handler)
	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	t.Cleanup(srv.Close)
}

func writeSuccessJSON(t *testing.T, w http.ResponseWriter, data map[string]any) {
	t.Helper()
	data["success"] = true
	if err := json.NewEncoder(w).Encode(data); err != nil {
		t.Fatalf("encode JSON response: %v", err)
	}
}
