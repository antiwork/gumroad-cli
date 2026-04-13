package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var (
	ErrSelectNoInput        = errors.New("selection required but --no-input is set")
	ErrSelectNonInteractive = errors.New("selection required but stdin is not interactive")
)

// SelectOption represents a single choice in a numbered menu.
type SelectOption struct {
	Label string
	Value string
}

// Select displays a numbered menu and returns the chosen option's Value.
// It respects --no-input and non-TTY stdin.
func Select(message string, options []SelectOption, in io.Reader, out io.Writer, noInput bool) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options to select from")
	}
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}
	if noInput {
		return "", fmt.Errorf("%w. Use a positional argument or flag instead", ErrSelectNoInput)
	}

	file, ok := in.(*os.File)
	if !ok || !isTerminal(int(file.Fd())) {
		return "", fmt.Errorf("%w. Use a positional argument or flag instead", ErrSelectNonInteractive)
	}

	fmt.Fprintln(out, message)
	for i, opt := range options {
		fmt.Fprintf(out, "  %d) %s\n", i+1, opt.Label)
	}
	fmt.Fprintf(out, "Choose [1-%d]: ", len(options))

	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || answer == "") {
		return "", err
	}

	choice, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || choice < 1 || choice > len(options) {
		return "", fmt.Errorf("invalid choice: %s", strings.TrimSpace(answer))
	}

	return options[choice-1].Value, nil
}
