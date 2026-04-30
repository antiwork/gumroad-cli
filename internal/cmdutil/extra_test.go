package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestOptionsFromNilCommand(t *testing.T) {
	got := OptionsFrom(nil)
	if got.Version != "dev" {
		t.Fatalf("got version %q, want dev", got.Version)
	}
	if got.Context == nil {
		t.Fatal("expected default context")
	}
}

func TestWithOptionsUsesParentContextWhenMissing(t *testing.T) {
	traceKey := contextKey("trace")
	parent := context.WithValue(context.Background(), traceKey, "abc")
	cmd := &cobra.Command{Use: "demo"}
	cmd.SetContext(WithOptions(parent, Options{}))

	got := OptionsFrom(cmd)
	if got.Context.Value(traceKey) != "abc" {
		t.Fatalf("expected inherited context value, got %v", got.Context.Value(traceKey))
	}
}

func TestOptionsOutAndErrPreferConfiguredWriters(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer

	opts := Options{Stdout: &out, Stderr: &errBuf}
	if opts.Out() != &out {
		t.Fatal("Out should return configured stdout writer")
	}
	if opts.Err() != &errBuf {
		t.Fatal("Err should return configured stderr writer")
	}
}

func TestConfirmAction_YesSkipsPrompt(t *testing.T) {
	opts := DefaultOptions()
	opts.Yes = true

	ok, err := ConfirmAction(opts, "Delete product prod_123?")
	if err != nil {
		t.Fatalf("ConfirmAction returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected yes flag to auto-confirm")
	}
}

func TestPrintDryRunRequest_GETNoOp(t *testing.T) {
	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintDryRunRequest(opts, "GET", "/user", nil); err != nil {
		t.Fatalf("PrintDryRunRequest returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output for GET dry-run, got %q", out.String())
	}
}

func TestPrintDryRunRequest_PlainOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := PrintDryRunRequest(opts, "DELETE", "/products/prod_123", url.Values{
		"zeta":  {"9"},
		"alpha": {"1", "2"},
	})
	if err != nil {
		t.Fatalf("PrintDryRunRequest returned error: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := "DELETE\t/products/prod_123\talpha=1,2&zeta=9"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrintDryRunRequest_StyledEscapesNewlinesInValues(t *testing.T) {
	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out

	err := PrintDryRunRequest(opts, "POST", "/users/create_comment", url.Values{
		"email":   {"u@example.com"},
		"content": {"line 1\nline 2"},
	})
	if err != nil {
		t.Fatalf("PrintDryRunRequest returned error: %v", err)
	}

	got := out.String()
	if strings.Count(got, "\n") != 3 {
		t.Errorf("styled dry-run preview must escape embedded newlines so multi-line values do not produce orphan lines without a key prefix; got %d lines:\n%q", strings.Count(got, "\n"), got)
	}
	if !strings.Contains(got, `content: line 1\nline 2`) {
		t.Errorf("expected content value to render with literal \\n escape, got: %q", got)
	}
}

func TestPrintDryRunRequest_PlainOutputRedactsSensitiveParams(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := PrintDryRunRequest(opts, "PUT", "/licenses/verify", url.Values{
		"license_key": {"SUPER-SECRET"},
		"product_id":  {"prod_123"},
	})
	if err != nil {
		t.Fatalf("PrintDryRunRequest returned error: %v", err)
	}

	got := strings.TrimSpace(out.String())
	if strings.Contains(got, "SUPER-SECRET") {
		t.Fatalf("expected sensitive value to be redacted, got %q", got)
	}
	if !strings.Contains(got, "license_key=REDACTED") {
		t.Fatalf("expected redacted license_key, got %q", got)
	}
}

func TestPrintDryRunRequest_JSONOutputRedactsSensitiveParams(t *testing.T) {
	opts := DefaultOptions()
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := PrintDryRunRequest(opts, "PUT", "/licenses/verify", url.Values{
		"license_key": {"SUPER-SECRET"},
		"product_id":  {"prod_123"},
	})
	if err != nil {
		t.Fatalf("PrintDryRunRequest returned error: %v", err)
	}

	if strings.Contains(out.String(), "SUPER-SECRET") {
		t.Fatalf("expected sensitive value to be redacted, got %q", out.String())
	}
	if !strings.Contains(out.String(), `"license_key": [`) || !strings.Contains(out.String(), `"REDACTED"`) {
		t.Fatalf("expected redacted JSON payload, got %q", out.String())
	}
}

func TestPrintDryRunAction_JSONOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintDryRunAction(opts, "remove stored API token"); err != nil {
		t.Fatalf("PrintDryRunAction returned error: %v", err)
	}

	var resp struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("dry-run action output is invalid JSON: %v\n%s", err, out.String())
	}
	if !resp.DryRun || resp.Action != "remove stored API token" {
		t.Fatalf("unexpected payload: %+v", resp)
	}
}

func TestPrintDryRunAction_PlainOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintDryRunAction(opts, "remove stored API token"); err != nil {
		t.Fatalf("PrintDryRunAction returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "remove stored API token" {
		t.Fatalf("unexpected plain output: %q", out.String())
	}
}

func TestPrintCancelledAction_JSONOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintCancelledAction(opts, "delete product prod_123", "prod_123"); err != nil {
		t.Fatalf("PrintCancelledAction returned error: %v", err)
	}

	var resp struct {
		Success   bool            `json:"success"`
		Cancelled bool            `json:"cancelled"`
		Message   string          `json:"message"`
		ID        string          `json:"id"`
		Action    string          `json:"action"`
		Result    json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("cancelled action output is invalid JSON: %v\n%s", err, out.String())
	}
	if resp.Success {
		t.Fatal("cancelled action should report success=false")
	}
	if !resp.Cancelled {
		t.Fatal("cancelled action should report cancelled=true")
	}
	if resp.ID != "prod_123" {
		t.Fatalf("unexpected id %q", resp.ID)
	}
	if resp.Message != "Cancelled: delete product prod_123." {
		t.Fatalf("unexpected message %q", resp.Message)
	}
	if resp.Action != "delete product prod_123" {
		t.Fatalf("unexpected action %q", resp.Action)
	}
	if strings.TrimSpace(string(resp.Result)) != "null" {
		t.Fatalf("unexpected result payload %q", resp.Result)
	}
}

func TestPrintCancelledAction_PlainOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintCancelledAction(opts, "delete product prod_123", "prod_123"); err != nil {
		t.Fatalf("PrintCancelledAction returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "false\tCancelled: delete product prod_123." {
		t.Fatalf("unexpected plain output: %q", out.String())
	}
}

func TestPrintCancelledAction_DefaultOutput(t *testing.T) {
	setColorEnabledForTest(t, false)

	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out

	if err := PrintCancelledAction(opts, "delete product prod_123", "prod_123"); err != nil {
		t.Fatalf("PrintCancelledAction returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "Cancelled: delete product prod_123." {
		t.Fatalf("unexpected default output: %q", out.String())
	}
}

func TestReplayCommandQuotesShellValues(t *testing.T) {
	got := ReplayCommand("gumroad sales list",
		CommandArg{Flag: "--all", Boolean: true},
		CommandArg{Flag: "--email", Value: "buyer name@example.com"},
	)
	want := "gumroad sales list --all --email 'buyer name@example.com'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCommandQuoterWindowsVariants(t *testing.T) {
	oldGOOS := hintGOOS
	oldGetenv := hintGetenv
	defer func() {
		hintGOOS = oldGOOS
		hintGetenv = oldGetenv
	}()

	hintGOOS = "windows"
	hintGetenv = func(key string) string {
		if key == "PSModulePath" {
			return "C:\\Users\\demo\\Documents\\PowerShell"
		}
		return ""
	}
	if got := commandQuoter()("a b's"); got != "'a b''s'" {
		t.Fatalf("unexpected PowerShell quoting: %q", got)
	}

	hintGetenv = func(key string) string {
		switch key {
		case "SHELL":
			return "C:\\Program Files\\PowerShell\\7\\pwsh.exe"
		case "PSModulePath":
			return "C:\\Users\\demo\\Documents\\PowerShell"
		default:
			return ""
		}
	}
	if got := commandQuoter()("a b's"); got != "'a b''s'" {
		t.Fatalf("unexpected PowerShell precedence with SHELL set: %q", got)
	}

	hintGetenv = func(string) string { return "" }
	if got := commandQuoter()(`a b"c`); got != `"a b""c"` {
		t.Fatalf("unexpected cmd.exe quoting: %q", got)
	}
	if looksLikePowerShell() {
		t.Fatal("expected looksLikePowerShell to be false without PowerShell-specific env vars")
	}
}

func TestShellQuoteHelpers(t *testing.T) {
	if got := shellQuotePOSIX("don't panic"); got != `'don'"'"'t panic'` {
		t.Fatalf("unexpected POSIX quoting: %q", got)
	}
	if got := shellQuotePowerShell("a b's"); got != "'a b''s'" {
		t.Fatalf("unexpected PowerShell quoting: %q", got)
	}
	if got := shellQuoteCmd(`a b"c`); got != `"a b""c"` {
		t.Fatalf("unexpected cmd.exe quoting: %q", got)
	}
	if got := shellQuotePOSIX("plain"); got != "plain" {
		t.Fatalf("unexpected unquoted POSIX output: %q", got)
	}
	if got := shellQuotePowerShell("plain"); got != "plain" {
		t.Fatalf("unexpected unquoted PowerShell output: %q", got)
	}
	if got := shellQuoteCmd("plain"); got != "plain" {
		t.Fatalf("unexpected unquoted cmd.exe output: %q", got)
	}
}

func TestRequirePercentFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Int("percent-off", 0, "")

	if err := RequirePercentFlag(cmd, "percent-off", 200); err != nil {
		t.Fatalf("unchanged flag should not error: %v", err)
	}

	if err := cmd.Flags().Set("percent-off", "200"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequirePercentFlag(cmd, "percent-off", 200); err == nil || !strings.Contains(err.Error(), "between 1 and 100") {
		t.Fatalf("expected range error, got %v", err)
	}

	cmd = &cobra.Command{Use: "demo"}
	cmd.Flags().Int("percent-off", 0, "")
	if err := cmd.Flags().Set("percent-off", "25"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequirePercentFlag(cmd, "percent-off", 25); err != nil {
		t.Fatalf("expected valid percent to pass, got %v", err)
	}
}

func TestRequireDateFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().String("before", "", "")

	if err := RequireDateFlag(cmd, "before", "not-a-date"); err != nil {
		t.Fatalf("unchanged flag should not error: %v", err)
	}

	if err := cmd.Flags().Set("before", "2024-13-01"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireDateFlag(cmd, "before", "2024-13-01"); err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("expected date format error, got %v", err)
	}

	cmd = &cobra.Command{Use: "demo"}
	cmd.Flags().String("before", "", "")
	if err := cmd.Flags().Set("before", "2024-12-01"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireDateFlag(cmd, "before", "2024-12-01"); err != nil {
		t.Fatalf("expected valid date to pass, got %v", err)
	}
}

func TestRequireHTTPURLFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().String("url", "", "")

	if err := cmd.Flags().Set("url", "/relative"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireHTTPURLFlag(cmd, "url", "/relative"); err == nil || !strings.Contains(err.Error(), "valid absolute URL") {
		t.Fatalf("expected absolute URL error, got %v", err)
	}

	cmd = &cobra.Command{Use: "demo"}
	cmd.Flags().String("url", "", "")
	if err := cmd.Flags().Set("url", "ftp://example.com/file"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireHTTPURLFlag(cmd, "url", "ftp://example.com/file"); err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("expected scheme error, got %v", err)
	}

	cmd = &cobra.Command{Use: "demo"}
	cmd.Flags().String("url", "", "")
	if err := cmd.Flags().Set("url", "https://example.com/hook"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireHTTPURLFlag(cmd, "url", "https://example.com/hook"); err != nil {
		t.Fatalf("expected valid URL to pass, got %v", err)
	}
}

func TestRunRequestWithSuccess_JSONOutput(t *testing.T) {
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeSuccessJSON(t, w, map[string]any{"variant": map[string]any{"id": "var_123"}})
	})

	opts := DefaultOptions()
	opts.JSONOutput = true
	opts.Version = "test"
	var out bytes.Buffer
	opts.Stdout = &out

	if err := RunRequestWithSuccess(opts, "Creating...", "POST", "/variants", nil, "var_123", "Variant created."); err != nil {
		t.Fatalf("RunRequestWithSuccess failed: %v", err)
	}

	var resp struct {
		Success bool                   `json:"success"`
		Message string                 `json:"message"`
		ID      string                 `json:"id"`
		Result  map[string]interface{} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("mutation JSON output is invalid: %v\n%s", err, out.String())
	}
	if !resp.Success || resp.Message != "Variant created." || resp.ID != "var_123" {
		t.Fatalf("unexpected mutation envelope: %+v", resp)
	}
	variant, ok := resp.Result["variant"].(map[string]interface{})
	if !ok || variant["id"] != "var_123" {
		t.Fatalf("unexpected mutation result: %+v", resp.Result)
	}
}

func TestRunRequestWithSuccess_JQOutput(t *testing.T) {
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeSuccessJSON(t, w, map[string]any{"variant": map[string]any{"id": "var_123"}})
	})

	opts := DefaultOptions()
	opts.JQExpr = ".message"
	opts.Version = "test"
	var out bytes.Buffer
	opts.Stdout = &out

	if err := RunRequestWithSuccess(opts, "Creating...", "POST", "/variants", nil, "var_123", "Variant created."); err != nil {
		t.Fatalf("RunRequestWithSuccess failed: %v", err)
	}

	if strings.TrimSpace(out.String()) != `"Variant created."` {
		t.Fatalf("unexpected jq output: %q", out.String())
	}
}

func TestRunRequestWithSuccess_PlainOutput(t *testing.T) {
	setupAuthedAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeSuccessJSON(t, w, map[string]any{})
	})

	opts := DefaultOptions()
	opts.PlainOutput = true
	opts.Version = "test"
	var out bytes.Buffer
	opts.Stdout = &out

	if err := RunRequestWithSuccess(opts, "Updating...", "PUT", "/licenses/enable", nil, "prod_abc", "License enabled."); err != nil {
		t.Fatalf("RunRequestWithSuccess failed: %v", err)
	}

	if strings.TrimSpace(out.String()) != "true\tLicense enabled." {
		t.Fatalf("unexpected plain output: %q", out.String())
	}
}

func TestRunRequestWithSuccess_DryRunSkipsAuth(t *testing.T) {
	opts := DefaultOptions()
	opts.DryRun = true
	var out bytes.Buffer
	opts.Stdout = &out

	if err := RunRequestWithSuccess(opts, "Deleting...", "DELETE", "/products/prod_123", nil, "prod_123", "Product deleted."); err != nil {
		t.Fatalf("RunRequestWithSuccess failed: %v", err)
	}

	if !strings.Contains(out.String(), "Dry run") || !strings.Contains(out.String(), "DELETE /products/prod_123") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}
}
