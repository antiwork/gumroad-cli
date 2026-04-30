package cmdutil

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type contextKey string

const optionsContextKey contextKey = "gumroad-cmd-options"

type Options struct {
	Context     context.Context
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	JSONOutput  bool
	PlainOutput bool
	JQExpr      string
	Quiet       bool
	DryRun      bool
	NoColor     bool
	NoInput     bool
	NoTUI       bool
	Yes         bool
	NoImage     bool
	PageDelay   time.Duration
	Debug       bool
	Version     string
}

func DefaultOptions() Options {
	return Options{
		Context: context.Background(),
		Version: "dev",
	}
}

func WithOptions(ctx context.Context, opts Options) context.Context {
	if opts.Context == nil {
		opts.Context = ctx
	}
	return context.WithValue(ctx, optionsContextKey, opts)
}

func OptionsFrom(cmd *cobra.Command) Options {
	if cmd == nil {
		return DefaultOptions()
	}

	ctx := cmd.Context()
	if ctx == nil {
		return DefaultOptions()
	}

	opts, ok := ctx.Value(optionsContextKey).(Options)
	if !ok {
		return DefaultOptions()
	}

	return opts
}

func (o Options) Out() io.Writer {
	if o.Stdout != nil {
		return o.Stdout
	}
	return os.Stdout
}

func (o Options) In() io.Reader {
	if o.Stdin != nil {
		return o.Stdin
	}
	return os.Stdin
}

func (o Options) Err() io.Writer {
	if o.Stderr != nil {
		return o.Stderr
	}
	return os.Stderr
}

func (o Options) Style() output.Styler {
	return output.NewStylerForWriter(o.Out(), o.NoColor)
}

// UsesJSONOutput reports whether the command should emit JSON output.
func (o Options) UsesJSONOutput() bool {
	return o.JSONOutput || o.JQExpr != ""
}

// DebugEnabled reports whether debug logging should be enabled.
func (o Options) DebugEnabled() bool {
	return o.Debug || os.Getenv("GUMROAD_DEBUG") == "1"
}

// InteractiveTUIAllowed reports whether a full-screen bubbletea TUI is safe to
// launch for this invocation. It returns false in any context that suggests a
// non-human caller — JSON/plain/jq output, --no-input, --no-tui, --quiet,
// non-TTY stdout, non-TTY stdin (piped input), NO_COLOR set, GUMROAD_TUI=0, or
// CI env vars. Commands MUST guard their TUI path with this — the regular
// non-TUI render must remain byte-for-byte identical for scripts and agents.
func (o Options) InteractiveTUIAllowed() bool {
	if o.NoTUI {
		return false
	}
	if o.UsesJSONOutput() {
		return false
	}
	if o.PlainOutput {
		return false
	}
	if o.NoInput {
		return false
	}
	if o.Quiet {
		return false
	}
	if !o.Style().Enabled() {
		return false
	}
	if !output.IsTTY() {
		return false
	}
	if !stdinIsTerminal(o.In()) {
		return false
	}
	if v := os.Getenv("GUMROAD_TUI"); v == "0" || v == "false" {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	return true
}

var stdinIsTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return output.IsFileTerminal(f)
}
