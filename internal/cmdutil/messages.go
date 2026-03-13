package cmdutil

import "github.com/antiwork/gumroad-cli/internal/output"

// PrintInfo writes a non-essential informational message unless quiet mode is enabled.
func PrintInfo(opts Options, message string) error {
	if opts.Quiet {
		return nil
	}
	return output.Writeln(opts.Out(), message)
}

// PrintSuccess writes a success message unless quiet mode is enabled.
func PrintSuccess(opts Options, message string) error {
	if opts.Quiet {
		return nil
	}
	style := opts.Style()
	return output.Writeln(opts.Out(), style.Green(message))
}
