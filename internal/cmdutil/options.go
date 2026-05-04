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
	Context        context.Context
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	JSONOutput     bool
	PlainOutput    bool
	JQExpr         string
	Quiet          bool
	DryRun         bool
	NoColor        bool
	NoInput        bool
	NonInteractive bool
	Yes            bool
	NoImage        bool
	PageDelay      time.Duration
	Debug          bool
	Version        string
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
