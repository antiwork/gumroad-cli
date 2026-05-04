package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

// Setup creates a mock HTTP server and temp config for command tests.
// Returns the server. Cleanup is automatic via t.Cleanup.
func Setup(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	cfgDir := t.TempDir()
	configDir := filepath.Join(cfgDir, "gumroad")
	configPath := filepath.Join(configDir, "config.json")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"access_token":"test-token"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv(config.EnvAccessToken, "")

	srv := httptest.NewServer(handler)
	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	t.Cleanup(srv.Close)
	return srv
}

// SetupAdmin creates a mock admin HTTP server and temp admin config for
// command tests. Cleanup is automatic via t.Cleanup.
func SetupAdmin(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(adminconfig.EnvAccessToken, "")
	t.Setenv(adminconfig.LegacyEnvAccessToken, "")

	if err := adminconfig.Save(&adminconfig.Config{
		Token: "admin-token",
		Actor: adminconfig.Actor{
			Name:  "Test Admin",
			Email: "admin@example.com",
		},
	}); err != nil {
		t.Fatalf("Save admin config failed: %v", err)
	}

	srv := httptest.NewServer(handler)
	t.Setenv(adminapi.EnvAPIBaseURL, srv.URL)
	t.Cleanup(srv.Close)
	return srv
}

// TestOptions returns the default per-command options used by command tests.
func TestOptions(mutators ...OptionsMutator) cmdutil.Options {
	opts := cmdutil.DefaultOptions()
	opts.Quiet = true
	opts.Version = "test"
	for _, mutate := range mutators {
		mutate(&opts)
	}
	return opts
}

type OptionsMutator func(*cmdutil.Options)

var captureMu sync.Mutex

func mustPipe() (*os.File, *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	return r, w
}

// WithOptions binds explicit options to a command for direct RunE/Execute tests.
func WithOptions(cmd *cobra.Command, opts cmdutil.Options) *cobra.Command {
	ctx := context.Background()
	if existing := cmd.Context(); existing != nil {
		ctx = existing
	}
	opts.Context = ctx
	if opts.Stdin != nil {
		cmd.SetIn(opts.Stdin)
	}
	if opts.Stdout != nil && !usesProcessFile(opts.Stdout, os.Stdout) {
		cmd.SetOut(opts.Stdout)
	}
	if opts.Stderr != nil && !usesProcessFile(opts.Stderr, os.Stderr) {
		cmd.SetErr(opts.Stderr)
	}
	cmd.SetContext(cmdutil.WithOptions(ctx, opts))
	return cmd
}

func usesProcessFile(w io.Writer, processFile *os.File) bool {
	file, ok := w.(*os.File)
	return ok && file == processFile
}

// Command applies test options and any requested mutations to a command.
func Command(cmd *cobra.Command, mutators ...OptionsMutator) *cobra.Command {
	return WithOptions(cmd, TestOptions(mutators...))
}

func JSONOutput() OptionsMutator {
	return func(opts *cmdutil.Options) { opts.JSONOutput = true }
}

func PlainOutput() OptionsMutator {
	return func(opts *cmdutil.Options) { opts.PlainOutput = true }
}

func JQ(expr string) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.JQExpr = expr }
}

func Quiet(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.Quiet = value }
}

func DryRun(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.DryRun = value }
}

func NoInput(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.NoInput = value }
}

func NonInteractive(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) {
		opts.NonInteractive = value
		if value {
			opts.NoInput = true
		}
	}
}

func NoColor(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.NoColor = value }
}

func Yes(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.Yes = value }
}

func NoImage(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.NoImage = value }
}

func Debug(value bool) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.Debug = value }
}

func SetColorEnabled(t *testing.T, enabled bool) {
	t.Helper()
	output.SetColorEnabledForTesting(enabled)
	t.Cleanup(output.ResetColorEnabledForTesting)
}

func SetStdoutIsTerminal(t *testing.T, enabled bool) {
	t.Helper()
	output.SetStdoutIsTerminalForTesting(enabled)
	t.Cleanup(output.ResetStdoutIsTerminalForTesting)
}

func AssertNoANSI(t *testing.T, value string) {
	t.Helper()
	if bytes.Contains([]byte(value), []byte("\033[")) {
		t.Fatalf("expected no ANSI output, got %q", value)
	}
}

func Stdout(w io.Writer) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.Stdout = w }
}

func Stdin(r io.Reader) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.Stdin = r }
}

func Stderr(w io.Writer) OptionsMutator {
	return func(opts *cmdutil.Options) { opts.Stderr = w }
}

// CaptureStdout captures os.Stdout output during fn execution.
// Access is serialized so parallel tests do not race on global os.Stdout.
func CaptureStdout(fn func()) string {
	captureMu.Lock()
	defer captureMu.Unlock()

	old := os.Stdout
	r, w := mustPipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	_ = r.Close()
	return buf.String()
}

// CaptureOutput captures both stdout and stderr during fn execution.
// Access is serialized so parallel tests do not race on global descriptors.
func CaptureOutput(fn func()) (string, string) {
	captureMu.Lock()
	defer captureMu.Unlock()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW := mustPipe()
	stderrR, stderrW := mustPipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(&stdoutBuf, stdoutR) }()
	go func() { defer wg.Done(); _, _ = io.Copy(&stderrBuf, stderrR) }()

	fn()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	wg.Wait()
	_ = stdoutR.Close()
	_ = stderrR.Close()

	return stdoutBuf.String(), stderrBuf.String()
}

// MustExecute runs a command in tests and fails immediately on error.
func MustExecute(t *testing.T, cmd interface{ Execute() error }) {
	t.Helper()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

// JSON encodes a success response.
func JSON(t *testing.T, w http.ResponseWriter, data map[string]any) {
	t.Helper()
	data["success"] = true
	if err := json.NewEncoder(w).Encode(data); err != nil {
		t.Fatalf("encode JSON response: %v", err)
	}
}

// RawJSON writes a literal JSON response body.
func RawJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write raw JSON: %v", err)
	}
}

// Fixture loads a test fixture from a path relative to the current package.
func Fixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(data)
}
