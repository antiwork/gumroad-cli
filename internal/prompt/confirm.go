package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	ErrConfirmationNoInput        = errors.New("confirmation required but interactive prompts are disabled")
	ErrConfirmationNonInteractive = errors.New("confirmation required but stdin is not interactive")
)

func Confirm(message string, in io.Reader, out io.Writer, yes, noInput bool) (bool, error) {
	if yes {
		return true, nil
	}
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}
	if noInput {
		return false, confirmationRequiredError(ErrConfirmationNoInput)
	}

	file, ok := in.(*os.File)
	if !ok || !isTerminal(int(file.Fd())) {
		return false, confirmationRequiredError(ErrConfirmationNonInteractive)
	}

	fmt.Fprintf(out, "%s [y/N] ", message)
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) || answer == "" {
			return false, err
		}
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func confirmationRequiredError(err error) error {
	return fmt.Errorf("%w. Use --yes to skip confirmation", err)
}
