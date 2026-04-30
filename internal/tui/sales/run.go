package sales

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the interactive sales browser. Callers MUST first verify they
// are in an interactive context with cmdutil.Options.InteractiveTUIAllowed();
// this function does not re-check.
//
// in/out should be the same TTY-backed file handles the rest of the CLI uses
// (typically os.Stdin/os.Stdout). Run blocks until the user quits.
func Run(in io.Reader, out io.Writer, sales []Sale) error {
	m := NewModel(sales)
	prog := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	_, err := prog.Run()
	return err
}
