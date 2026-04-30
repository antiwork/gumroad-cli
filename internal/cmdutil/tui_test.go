package cmdutil

import (
	"io"
	"os"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/output"
)

func withFakeTTYBoth(t *testing.T) {
	t.Helper()
	output.SetStdoutIsTerminalForTesting(true)
	output.SetColorEnabledForTesting(true)
	origStdin := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		output.ResetStdoutIsTerminalForTesting()
		output.ResetColorEnabledForTesting()
		stdinIsTerminal = origStdin
	})
}

func baseOpts() Options {
	o := DefaultOptions()
	o.Stdout = os.Stdout
	o.Stdin = os.Stdin
	return o
}

func TestInteractiveTUIAllowed_GreenPath(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("GUMROAD_TUI", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	if !baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected TUI to be allowed when every gate passes")
	}
}

func TestInteractiveTUIAllowed_BlocksJSON(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("GUMROAD_TUI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	o := baseOpts()
	o.JSONOutput = true
	if o.InteractiveTUIAllowed() {
		t.Fatal("expected --json to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksJQ(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	o := baseOpts()
	o.JQExpr = ".sales[0].id"
	if o.InteractiveTUIAllowed() {
		t.Fatal("expected --jq to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksPlain(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	o := baseOpts()
	o.PlainOutput = true
	if o.InteractiveTUIAllowed() {
		t.Fatal("expected --plain to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksNoInput(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	o := baseOpts()
	o.NoInput = true
	if o.InteractiveTUIAllowed() {
		t.Fatal("expected --no-input to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksNoTUIFlag(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	o := baseOpts()
	o.NoTUI = true
	if o.InteractiveTUIAllowed() {
		t.Fatal("expected --no-tui to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksQuiet(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)

	o := baseOpts()
	o.Quiet = true
	if o.InteractiveTUIAllowed() {
		t.Fatal("expected --quiet to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksNonTTYStdout(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	output.SetColorEnabledForTesting(true)
	output.SetStdoutIsTerminalForTesting(false)
	t.Cleanup(output.ResetColorEnabledForTesting)
	t.Cleanup(output.ResetStdoutIsTerminalForTesting)
	origStdin := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = origStdin })

	if baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected non-TTY stdout to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksNonTTYStdin(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	withFakeTTYBoth(t)
	stdinIsTerminal = func(io.Reader) bool { return false }

	if baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected piped stdin to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksNoColor(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "xterm-256color")
	output.SetStdoutIsTerminalForTesting(true)
	output.SetColorEnabledForTesting(false)
	t.Cleanup(output.ResetColorEnabledForTesting)
	t.Cleanup(output.ResetStdoutIsTerminalForTesting)
	origStdin := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = origStdin })

	if baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected NO_COLOR / disabled color to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksOnCI(t *testing.T) {
	withFakeTTYBoth(t)
	t.Setenv("CI", "true")

	if baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected CI=true to block TUI")
	}
}

func TestInteractiveTUIAllowed_BlocksOnGumroadTUIZero(t *testing.T) {
	withFakeTTYBoth(t)
	t.Setenv("CI", "")
	t.Setenv("GUMROAD_TUI", "0")

	if baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected GUMROAD_TUI=0 to block TUI")
	}

	t.Setenv("GUMROAD_TUI", "false")
	if baseOpts().InteractiveTUIAllowed() {
		t.Fatal("expected GUMROAD_TUI=false to block TUI")
	}
}

func TestStdinIsTerminal_NonFileReturnsFalse(t *testing.T) {
	if stdinIsTerminal(io.NopCloser(nil)) {
		t.Fatal("non-file reader must not register as a terminal")
	}
}
