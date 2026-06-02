package pageutil

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

const DefaultHTMLPath = "./landing.html"

type HTMLInput struct {
	Source string
	HTML   string
}

func ReadHTML(r io.Reader, path string) (HTMLInput, error) {
	if path == "" {
		path = DefaultHTMLPath
	}

	if path == "-" {
		data, err := io.ReadAll(r)
		if err != nil {
			return HTMLInput{}, fmt.Errorf("cannot read stdin: %w", err)
		}
		return HTMLInput{Source: "stdin", HTML: string(data)}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return HTMLInput{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return HTMLInput{Source: path, HTML: string(data)}, nil
}

func StripTerminalControls(value string) string {
	var b strings.Builder
	b.Grow(len(value))

	for i := 0; i < len(value); {
		r, size := utf8.DecodeRuneInString(value[i:])

		if r == '\x1b' {
			i += skipEscapeSequence(value[i:])
			continue
		}
		if r < 0x20 || r == 0x7f || unicode.IsControl(r) {
			i += size
			continue
		}

		b.WriteRune(r)
		i += size
	}

	return b.String()
}

func skipEscapeSequence(value string) int {
	if len(value) < 2 {
		return 1
	}

	switch value[1] {
	case '[':
		for i := 2; i < len(value); i++ {
			if value[i] >= 0x40 && value[i] <= 0x7e {
				return i + 1
			}
		}
		return len(value)
	case ']':
		for i := 2; i < len(value); i++ {
			if value[i] == '\a' {
				return i + 1
			}
			if value[i] == '\x1b' && i+1 < len(value) && value[i+1] == '\\' {
				return i + 2
			}
		}
		return len(value)
	default:
		return 2
	}
}
