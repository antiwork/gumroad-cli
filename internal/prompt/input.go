package prompt

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	maxSecretInputBytes = 8 * 1024
	maxTokenInputBytes  = maxSecretInputBytes
)

var isTerminal = func(fd int) bool {
	return term.IsTerminal(fd)
}

// IsInteractive reports whether r is an *os.File connected to a terminal.
// Returns false if r is not an *os.File.
var IsInteractive = func(r io.Reader) bool {
	file, ok := r.(*os.File)
	return ok && isTerminal(int(file.Fd()))
}

var readPassword = func(fd int) ([]byte, error) {
	return term.ReadPassword(fd)
}

func SecretInput(promptLabel, noun string, in io.Reader, out io.Writer, noInput bool, noInputHint string) (string, error) {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}

	if file, ok := in.(*os.File); ok && isTerminal(int(file.Fd())) {
		if noInput {
			return "", fmt.Errorf("%s required but --no-input is set. %s", noun, noInputHint)
		}

		fmt.Fprintf(out, "Enter %s: ", promptLabel)
		secret, err := readPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("could not read %s: %w", noun, err)
		}
		return strings.TrimSpace(string(secret)), nil
	}

	data, err := io.ReadAll(io.LimitReader(in, maxSecretInputBytes+1))
	if err != nil {
		return "", fmt.Errorf("could not read %s from stdin: %w", noun, err)
	}
	if len(data) > maxSecretInputBytes {
		return "", fmt.Errorf("%s from stdin is too large (limit %d bytes)", noun, maxSecretInputBytes)
	}
	return strings.TrimSpace(string(data)), nil
}

func TokenInput(in io.Reader, out io.Writer, noInput bool) (string, error) {
	return SecretInput("your Gumroad API token", "token", in, out, noInput, "Pipe your token via stdin: echo $TOKEN | gumroad auth login")
}
