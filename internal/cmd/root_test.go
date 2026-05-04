package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/spf13/cobra"
)

func usageTestCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "gumroad user",
		Example: "  gumroad user",
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func stubCommand(runErr error) *cobra.Command {
	return &cobra.Command{
		Use:           "gumroad",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			return runErr
		},
	}
}

func replaceRootCommandFactory(t *testing.T, factory func() *cobra.Command) {
	t.Helper()

	previousFactory := newRootCommand
	newRootCommand = factory
	t.Cleanup(func() {
		newRootCommand = previousFactory
	})
}

func replaceGetOSArgs(t *testing.T, args []string) {
	t.Helper()
	previous := getOSArgs
	getOSArgs = func() []string { return args }
	t.Cleanup(func() { getOSArgs = previous })
}

func replaceExitProcess(t *testing.T, exitFn func(int)) {
	t.Helper()

	previousExit := exitProcess
	exitProcess = exitFn
	t.Cleanup(func() {
		exitProcess = previousExit
	})
}

func TestValidateOutputFlags_AllowsJSONAndJQ(t *testing.T) {
	opts := cmdutil.DefaultOptions()
	opts.JSONOutput = true
	opts.JQExpr = ".user.email"

	if err := validateOutputFlags(usageTestCommand(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCmd_RejectsPlainAndJSON(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"user", "--plain", "--json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected conflicting flag error")
	}
	if !strings.Contains(err.Error(), "--plain cannot be combined with --json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCmd_RejectsPlainAndJQ(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"user", "--plain", "--jq", ".user.email"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected conflicting flag error")
	}
	if !strings.Contains(err.Error(), "--plain cannot be combined with --jq") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputFlags_RejectsPlainJSONAndJQ(t *testing.T) {
	opts := cmdutil.DefaultOptions()
	opts.PlainOutput = true
	opts.JSONOutput = true
	opts.JQExpr = ".user.email"

	err := validateOutputFlags(usageTestCommand(), opts)
	if err == nil {
		t.Fatal("expected conflicting flag error")
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T", err)
	}
	if !strings.Contains(err.Error(), "--plain cannot be combined with --json or --jq") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCmd_RejectsNegativePageDelay(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"user", "--page-delay=-1s"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected negative page-delay error")
	}
	if !strings.Contains(err.Error(), "--page-delay cannot be negative") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCmd_HelpIncludesDryRunFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--help"})

	var out strings.Builder
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "--dry-run") {
		t.Fatalf("expected --dry-run in help output, got %q", out.String())
	}
}

func TestRootCmd_HelpIncludesNonInteractiveFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--help"})

	var out strings.Builder
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "--non-interactive") {
		t.Fatalf("expected --non-interactive in help output, got %q", out.String())
	}
}

func TestRootCmd_HelpIncludesPageDelayFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--help"})

	var out strings.Builder
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "--page-delay") {
		t.Fatalf("expected --page-delay in help output, got %q", out.String())
	}
}

func TestRootCmd_VersionFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--version"})

	var out strings.Builder
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "gumroad version ") {
		t.Fatalf("expected version output, got %q", out.String())
	}
}

func TestExecuteCommand_Success(t *testing.T) {
	cmd := stubCommand(nil)
	cmd.SetArgs([]string{})

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 0 {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteCommand_Error(t *testing.T) {
	cmd := stubCommand(errors.New("boom"))
	cmd.SetArgs([]string{})

	var stderr bytes.Buffer
	if code := executeCommand(cmd, &bytes.Buffer{}, &stderr); code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Error: boom") {
		t.Fatalf("expected formatted error, got %q", stderr.String())
	}
}

func TestExecuteCommand_JSONError(t *testing.T) {
	cmd := stubCommand(errors.New("boom"))
	cmd.SetArgs([]string{})

	opts := cmdutil.DefaultOptions()
	opts.JSONOutput = true
	cmd.SetContext(cmdutil.WithOptions(context.Background(), opts))

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got error %v with %q", err, stdout.String())
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if payload.Error.Type != "internal_error" {
		t.Fatalf("got error type %q, want internal_error", payload.Error.Type)
	}
	if payload.Error.Message != "boom" {
		t.Fatalf("got error message %q, want boom", payload.Error.Message)
	}
}

func TestExecuteCommand_JSONFlagConflictIsUsageError(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"user", "--plain", "--json"})

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got error %v with %q", err, stdout.String())
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if payload.Error.Type != "usage_error" || payload.Error.Code != "invalid_input" {
		t.Fatalf("unexpected structured error: %+v", payload.Error)
	}
	if !strings.Contains(payload.Error.Message, "--plain cannot be combined with --json") {
		t.Fatalf("unexpected error message %q", payload.Error.Message)
	}
}

func TestExecuteCommand_JSONNoInputConfirmationIsUsageError(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"auth", "logout", "--json", "--no-input"})

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got error %v with %q", err, stdout.String())
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if payload.Error.Type != "usage_error" || payload.Error.Code != "invalid_input" {
		t.Fatalf("unexpected structured error: %+v", payload.Error)
	}
	if payload.Error.Message != "confirmation required but interactive prompts are disabled. Use --yes to skip confirmation" {
		t.Fatalf("unexpected error message %q", payload.Error.Message)
	}
}

func TestExecuteCommand_JSONNonInteractiveConfirmationIsUsageError(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"auth", "logout", "--json", "--non-interactive"})

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got error %v with %q", err, stdout.String())
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if payload.Error.Type != "usage_error" || payload.Error.Code != "invalid_input" {
		t.Fatalf("unexpected structured error: %+v", payload.Error)
	}
	if payload.Error.Message != "confirmation required but interactive prompts are disabled. Use --yes to skip confirmation" {
		t.Fatalf("unexpected error message %q", payload.Error.Message)
	}
}

func TestExecuteCommand_BrokenPipe(t *testing.T) {
	cmd := stubCommand(io.ErrClosedPipe)
	cmd.SetArgs([]string{})

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 0 {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteCommand_WrappedBrokenPipe(t *testing.T) {
	cmd := stubCommand(fmt.Errorf("write failed: %w", io.ErrClosedPipe))
	cmd.SetArgs([]string{})

	var stdout, stderr bytes.Buffer
	if code := executeCommand(cmd, &stdout, &stderr); code != 0 {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExitCodeForCommandError_StructuredOutputBrokenPipe(t *testing.T) {
	cmd := stubCommand(nil)
	opts := cmdutil.DefaultOptions()
	opts.JSONOutput = true
	cmd.SetContext(cmdutil.WithOptions(context.Background(), opts))
	cmd.SetOut(failingWriter{err: io.ErrClosedPipe})

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if code := exitCodeForCommandError(cmd, errors.New("boom")); code != 0 {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestExitCodeForCommandError_StructuredOutputWriteFailureFallsBackToHumanError(t *testing.T) {
	cmd := stubCommand(nil)
	opts := cmdutil.DefaultOptions()
	opts.JSONOutput = true
	cmd.SetContext(cmdutil.WithOptions(context.Background(), opts))
	cmd.SetOut(failingWriter{err: errors.New("write failed")})

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if code := exitCodeForCommandError(cmd, errors.New("boom")); code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Error: write failed") {
		t.Fatalf("expected fallback human error, got %q", stderr.String())
	}
}

func TestExecute_UsesInjectedRootCommandAndExit(t *testing.T) {
	replaceRootCommandFactory(t, func() *cobra.Command {
		cmd := stubCommand(nil)
		cmd.SetArgs([]string{})
		return cmd
	})

	exitCode := -1
	replaceExitProcess(t, func(code int) {
		exitCode = code
	})

	Execute()
	if exitCode != 0 {
		t.Fatalf("got exit code %d, want 0", exitCode)
	}
}

func TestExecuteRootCommand_UsesInjectedRootCommand(t *testing.T) {
	replaceRootCommandFactory(t, func() *cobra.Command {
		cmd := stubCommand(nil)
		cmd.SetArgs([]string{})
		return cmd
	})

	if code := executeRootCommand(&bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("got exit code %d, want 0", code)
	}
}

func TestRootCmd_CustomFieldsUpdateHelpUsesRelevantExample(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"custom-fields", "update", "--help"})

	var out strings.Builder
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := out.String()
	if strings.Contains(text, "gumroad custom-fields list") || strings.Contains(text, "gumroad custom-fields create") {
		t.Fatalf("help should not include unrelated examples, got %q", text)
	}
	if strings.Contains(text, "gumroad custom-fields update --name <value> --product <value> --required") {
		t.Fatalf("help example should not include optional flags, got %q", text)
	}
	for _, want := range []string{"Examples:", "gumroad custom-fields update", "--name <value>", "--product <value>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in help output %q", want, text)
		}
	}
}

func TestNoColorRequested_FromValidationErrorContext(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"user", "--plain", "--json", "--no-color"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected conflicting flag error")
	}
	if !noColorRequested(cmd) {
		t.Fatal("expected --no-color to be preserved after validation error")
	}
}

func TestNoColorRequested_FromParsedFlagsWithoutContext(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--no-color", "unknown-command"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	if !noColorRequestedInArgs([]string{"--no-color", "unknown-command"}) {
		t.Fatal("expected --no-color to be detected from parsed flags without context")
	}
}

func TestNoColorRequested_FromContextOptions(t *testing.T) {
	cmd := NewRootCmd()
	opts := cmdutil.DefaultOptions()
	opts.NoColor = true
	cmd.SetContext(cmdutil.WithOptions(context.Background(), opts))

	if !noColorRequestedFromCommand(cmd) {
		t.Fatal("expected noColorRequestedFromCommand to honor context options")
	}
}

func TestNoColorRequestedInArgs_ParsesExplicitValues(t *testing.T) {
	if !noColorRequestedInArgs([]string{"--no-color=true"}) {
		t.Fatal("expected --no-color=true to be detected")
	}
	if noColorRequestedInArgs([]string{"--no-color=false"}) {
		t.Fatal("expected --no-color=false to be ignored")
	}
}

func TestStructuredOutputRequestedInArgs_ParsesJSONAndJQ(t *testing.T) {
	for _, args := range [][]string{
		{"--json"},
		{"user", "--json=true"},
		{"sales", "list", "--jq", ".sales[0].id"},
		{"sales", "list", "--jq=.sales[0].id"},
	} {
		if !structuredOutputRequestedInArgs(args) {
			t.Fatalf("expected structured output for args %v", args)
		}
	}

	if structuredOutputRequestedInArgs([]string{"--json=false"}) {
		t.Fatal("expected --json=false to be ignored")
	}
}

func TestRootCmd_FlagParseErrorIsUsageError(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--bogus"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected parse error")
	}

	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T", err)
	}
	if !strings.Contains(err.Error(), "unknown flag: --bogus") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommandContext_NilCommandUsesBackground(t *testing.T) {
	if commandContext(nil) == nil {
		t.Fatal("expected background context")
	}
}

func TestCommandContext_PrefersCommandContext(t *testing.T) {
	type contextKey string

	cmd := &cobra.Command{Use: "gumroad"}
	ctx := context.WithValue(context.Background(), contextKey("trace"), "abc123")
	cmd.SetContext(ctx)

	if got := commandContext(cmd).Value(contextKey("trace")); got != "abc123" {
		t.Fatalf("got context value %v, want abc123", got)
	}
}

func TestClassifyCommandError_Hint(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"api_error_with_hint", &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin}, api.HintRunAuthLogin},
		{"api_error_no_hint", &api.APIError{StatusCode: 500, Message: "Server error"}, ""},
		{"config_not_authenticated", config.ErrNotAuthenticated, api.HintRunAuthLogin},
		{"wrapped_api_error", fmt.Errorf("invalid token: %w", &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin}), api.HintRunAuthLogin},
		{"wrapped_config_error", fmt.Errorf("setup failed: %w", config.ErrNotAuthenticated), api.HintRunAuthLogin},
		{"wrapped_api_not_authenticated", fmt.Errorf("check failed: %w", api.ErrNotAuthenticated), api.HintRunAuthLogin},
		{"config_auth_with_remediation", fmt.Errorf("%w. Run `gumroad auth login` first or set `GUMROAD_ACCESS_TOKEN`", config.ErrNotAuthenticated), ""},
		{"plain_error", errors.New("boom"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCommandError(tt.err).Hint; got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintHumanCommandError_ShowsUploadRecoveryHint(t *testing.T) {
	output.SetColorEnabledForTesting(false)
	defer output.ResetColorEnabledForTesting()

	cmd := &cobra.Command{Use: "test"}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	state := &upload.UnknownStateError{
		UploadID: "up-1",
		Key:      "attachments/u/k/original/p.bin",
		Cause:    errors.New("503"),
	}
	printHumanCommandError(cmd, state)

	got := stderr.String()
	if !strings.Contains(got, "Hint:") || !strings.Contains(got, "Do not retry blindly") {
		t.Errorf("expected upload hint in human output, got:\n%s", got)
	}
	if !strings.Contains(got, "Recovery:") {
		t.Errorf("expected recovery block, got:\n%s", got)
	}
}

func TestPrintHumanCommandError_ShowsHint(t *testing.T) {
	output.SetColorEnabledForTesting(false)
	defer output.ResetColorEnabledForTesting()

	cmd := &cobra.Command{Use: "test"}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	printHumanCommandError(cmd, &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin})

	got := stderr.String()
	if !strings.Contains(got, "Error: Not authenticated.") {
		t.Errorf("expected error message, got %q", got)
	}
	if !strings.Contains(got, "Hint: Run: gumroad auth login") {
		t.Errorf("expected hint line, got %q", got)
	}
}

func TestPrintHumanCommandError_NoHint(t *testing.T) {
	output.SetColorEnabledForTesting(false)
	defer output.ResetColorEnabledForTesting()

	cmd := &cobra.Command{Use: "test"}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	printHumanCommandError(cmd, errors.New("something broke"))

	got := stderr.String()
	if strings.Contains(got, "Hint:") {
		t.Errorf("expected no hint line, got %q", got)
	}
}

func TestIsUnknownShorthandError(t *testing.T) {
	tests := []struct {
		err  string
		want bool
	}{
		{"unknown shorthand flag: 'c' in -cGk", true},
		{"unknown flag: --bogus", false},
		{"missing required argument: <id>", false},
	}
	for _, tt := range tests {
		if got := isUnknownShorthandError(errors.New(tt.err)); got != tt.want {
			t.Errorf("isUnknownShorthandError(%q) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestInsertDoubleDashBeforeArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		err  error
		want []string
	}{
		{
			"simple dash ID",
			[]string{"products", "view", "-cGk=="},
			errors.New("unknown shorthand flag: 'c' in -cGk=="),
			[]string{"products", "view", "--", "-cGk=="},
		},
		{
			"dash ID with flags before",
			[]string{"products", "update", "--name", "x", "-cGk=="},
			errors.New("unknown shorthand flag: 'c' in -cGk=="),
			[]string{"products", "update", "--name", "x", "--", "-cGk=="},
		},
		{
			"dash ID with flags after",
			[]string{"products", "view", "-cGk==", "--no-color"},
			errors.New("unknown shorthand flag: 'c' in -cGk=="),
			[]string{"products", "view", "--no-color", "--", "-cGk=="},
		},
		{
			"error with usage text appended",
			[]string{"products", "view", "-cGk=="},
			errors.New("unknown shorthand flag: 'c' in -cGk==\n\nUsage:\n  gumroad products view <id>"),
			[]string{"products", "view", "--", "-cGk=="},
		},
		{
			"no in keyword",
			[]string{"products", "view", "-cGk=="},
			errors.New("some other error"),
			nil,
		},
		{
			"offending arg not found",
			[]string{"products", "view", "abc123"},
			errors.New("unknown shorthand flag: 'c' in -cGk=="),
			nil,
		},
		{
			"short flag typo not retried",
			[]string{"products", "list", "-z"},
			errors.New("unknown shorthand flag: 'z' in -z"),
			nil,
		},
		{
			"two-char flag typo not retried",
			[]string{"products", "list", "-zx"},
			errors.New("unknown shorthand flag: 'z' in -zx"),
			nil,
		},
		{
			"alpha-only flag typo not retried",
			[]string{"products", "list", "-json"},
			errors.New("unknown shorthand flag: 'j' in -json"),
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insertDoubleDashBeforeArg(tt.args, tt.err)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d]=%q, want %q (full: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestExecuteRootCommand_RetriesDashPrefixedID(t *testing.T) {
	var gotArgs []string
	simArgs := []string{"gumroad", "view", "-dAsh1D=="}

	replaceRootCommandFactory(t, func() *cobra.Command {
		cmd := &cobra.Command{
			Use:           "gumroad",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
			return cmdutil.NewUsageError(c, err.Error())
		})
		sub := &cobra.Command{
			Use:  "view <id>",
			Args: cmdutil.ExactArgs(1),
			RunE: func(c *cobra.Command, args []string) error {
				gotArgs = args
				return nil
			},
		}
		cmd.AddCommand(sub)
		cmd.SetArgs(simArgs[1:]) // simulate os.Args[1:]
		return cmd
	})

	replaceGetOSArgs(t, simArgs)

	var stdout, stderr bytes.Buffer
	code := executeRootCommand(&stdout, &stderr)
	if code != 0 {
		t.Fatalf("got exit code %d, want 0; stderr=%q", code, stderr.String())
	}
	if len(gotArgs) != 1 || gotArgs[0] != "-dAsh1D==" {
		t.Fatalf("RunE got args %v, want [\"-dAsh1D==\"]", gotArgs)
	}
}

func TestExecuteRootCommand_DoesNotRetryShortFlagTypo(t *testing.T) {
	simArgs := []string{"gumroad", "view", "-z"}

	replaceRootCommandFactory(t, func() *cobra.Command {
		cmd := &cobra.Command{
			Use:           "gumroad",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
			return cmdutil.NewUsageError(c, err.Error())
		})
		sub := &cobra.Command{
			Use:  "view <id>",
			Args: cmdutil.ExactArgs(1),
			RunE: func(c *cobra.Command, args []string) error {
				t.Fatal("RunE should not be called for a flag typo")
				return nil
			},
		}
		cmd.AddCommand(sub)
		cmd.SetArgs(simArgs[1:])
		return cmd
	})

	replaceGetOSArgs(t, simArgs)

	var stdout, stderr bytes.Buffer
	code := executeRootCommand(&stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for flag typo")
	}
}
