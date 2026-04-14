package cmdutil

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type UsageError struct {
	Message string
}

func (e *UsageError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// UsageErrorf returns a human-facing usage error with concise help.
func UsageErrorf(cmd *cobra.Command, format string, args ...any) error {
	return NewUsageError(cmd, fmt.Sprintf(format, args...))
}

// NewUsageError returns a human-facing usage error with concise help.
func NewUsageError(cmd *cobra.Command, message string) error {
	var b strings.Builder

	b.WriteString(message)
	fmt.Fprintf(&b, "\n\nUsage:\n  %s", cmd.UseLine())

	if example := commandExample(cmd); example != "" {
		fmt.Fprintf(&b, "\n\nExamples:\n%s", example)
	}

	fmt.Fprintf(&b, "\n\nRun \"%s --help\" for more information.", cmd.CommandPath())
	return &UsageError{Message: b.String()}
}

// MissingFlagError returns a usage error for a missing required flag.
func MissingFlagError(cmd *cobra.Command, flag string) error {
	return UsageErrorf(cmd, "missing required flag: %s", flag)
}

// RequireAnyFlagChanged returns a usage error when none of the supplied flags were set.
func RequireAnyFlagChanged(cmd *cobra.Command, flags ...string) error {
	for _, flag := range flags {
		if cmd.Flags().Changed(flag) {
			return nil
		}
	}
	return UsageErrorf(cmd, "at least one field to update must be provided")
}

// ExactArgs validates an exact number of positional arguments with usage help.
func ExactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == n {
			return nil
		}
		if len(args) < n {
			required := positionalArgs(cmd.Use)
			if len(required) > len(args) {
				return UsageErrorf(cmd, "missing required argument: %s", required[len(args)])
			}
			return UsageErrorf(cmd, "missing required argument")
		}
		return UsageErrorf(cmd, "unexpected argument: %s", args[n])
	}
}

// PropagateExamples fills empty command examples from the nearest ancestor.
func PropagateExamples(cmd *cobra.Command) {
	propagateExamples(cmd, "")
}

func commandExample(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if example := strings.TrimSpace(c.Example); example != "" {
			return c.Example
		}
	}
	return ""
}

func propagateExamples(cmd *cobra.Command, inherited string) {
	leaf := len(cmd.Commands()) == 0
	example := inherited
	if current := strings.TrimSpace(cmd.Example); current != "" {
		example = cmd.Example
	} else if example != "" {
		cmd.Example = filteredExample(example, cmd.CommandPath())
		if strings.TrimSpace(cmd.Example) == "" && leaf {
			cmd.Example = generatedExample(cmd)
		}
		example = cmd.Example
	} else if leaf {
		cmd.Example = generatedExample(cmd)
		example = cmd.Example
	}

	for _, child := range cmd.Commands() {
		propagateExamples(child, example)
	}
}

func filteredExample(example string, commandPath string) string {
	var lines []string
	for _, line := range strings.Split(example, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, commandPath) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func positionalArgs(use string) []string {
	var args []string
	for _, field := range strings.Fields(use) {
		if strings.HasPrefix(field, "<") && strings.HasSuffix(field, ">") {
			args = append(args, field)
		}
	}
	return args
}

func generatedExample(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}

	var parts []string
	parts = append(parts, cmd.CommandPath())
	parts = append(parts, positionalArgs(cmd.Use)...)

	cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if !isRequiredExampleFlag(flag) {
			return
		}

		parts = append(parts, "--"+flag.Name)
		if flag.Value.Type() != "bool" {
			parts = append(parts, "<value>")
		}
	})

	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, " ")
}

func isRequiredExampleFlag(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	return strings.Contains(strings.ToLower(flag.Usage), "(required)")
}
