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

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
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
	if payload.Error.Message != "confirmation required but --no-input is set. Use --yes to skip confirmation" {
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
