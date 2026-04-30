package sales

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

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
