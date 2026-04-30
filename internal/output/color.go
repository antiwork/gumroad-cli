package output

import (
	"io"
	"os"
	"sync/atomic"

	"golang.org/x/term"
)

type Styler struct {
	enabled bool
}

var stdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

const (
	colorOverrideUnset int32 = -1
	colorOverrideOff   int32 = 0
	colorOverrideOn    int32 = 1
)

var colorEnabledOverride atomic.Int32

func init() {
	colorEnabledOverride.Store(colorOverrideUnset)
}

func IsTTY() bool {
	return stdoutIsTerminal()
}

func IsFileTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func NewStyler(noColor bool) Styler {
	return NewStylerForWriter(os.Stdout, noColor)
}

func NewStylerForWriter(w io.Writer, noColor bool) Styler {
	if enabled, ok := currentColorEnabledOverride(); ok {
		return Styler{enabled: enabled}
	}
	return Styler{enabled: shouldEnableColor(w, noColor)}
}

func (s Styler) Enabled() bool {
	return s.enabled
}

func shouldEnableColor(w io.Writer, noColor bool) bool {
	return isTerminalWriter(w) && os.Getenv("NO_COLOR") == "" && !isDumbTerminal() && !noColor
}

func isDumbTerminal() bool {
	return os.Getenv("TERM") == "dumb"
}

// SetColorEnabledForTesting forces color on or off regardless of runtime
// writer and terminal detection. This is a testing seam.
func SetColorEnabledForTesting(enabled bool) {
	if enabled {
		colorEnabledOverride.Store(colorOverrideOn)
		return
	}
	colorEnabledOverride.Store(colorOverrideOff)
}

// ResetColorEnabledForTesting clears the test-only color override.
func ResetColorEnabledForTesting() {
	colorEnabledOverride.Store(colorOverrideUnset)
}

// SetStdoutIsTerminalForTesting overrides stdout TTY detection for tests.
func SetStdoutIsTerminalForTesting(enabled bool) {
	stdoutIsTerminal = func() bool { return enabled }
}

// ResetStdoutIsTerminalForTesting restores default stdout TTY detection.
func ResetStdoutIsTerminalForTesting() {
	stdoutIsTerminal = func() bool {
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
}

func currentColorEnabledOverride() (bool, bool) {
	switch colorEnabledOverride.Load() {
	case colorOverrideOn:
		return true, true
	case colorOverrideOff:
		return false, true
	default:
		return false, false
	}
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	if file == os.Stdout {
		return stdoutIsTerminal()
	}
	return term.IsTerminal(int(file.Fd()))
}

func (s Styler) wrap(code, value string) string {
	if !s.enabled {
		return value
	}
	return code + value + "\033[0m"
}

func (s Styler) Bold(value string) string {
	return s.wrap("\033[1m", value)
}

func (s Styler) Red(value string) string {
	return s.wrap("\033[31m", value)
}

func (s Styler) Green(value string) string {
	return s.wrap("\033[32m", value)
}

func (s Styler) Yellow(value string) string {
	return s.wrap("\033[33m", value)
}

func (s Styler) Dim(value string) string {
	return s.wrap("\033[2m", value)
}

func Bold(s string) string {
	return NewStyler(false).Bold(s)
}

func Red(s string) string {
	return NewStyler(false).Red(s)
}

func Green(s string) string {
	return NewStyler(false).Green(s)
}

func Yellow(s string) string {
	return NewStyler(false).Yellow(s)
}

func Dim(s string) string {
	return NewStyler(false).Dim(s)
}
