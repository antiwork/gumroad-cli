package output

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

func PrintPlain(w io.Writer, rows [][]string) error {
	for _, row := range rows {
		escaped := make([]string, len(row))
		for i, cell := range row {
			escaped[i] = EscapePlainField(cell)
		}
		if _, err := fmt.Fprintln(w, strings.Join(escaped, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// EscapePlainField escapes control characters in value so it can be rendered
// safely on a single line. Used by both --plain output and the styled
// --dry-run preview so an embedded newline does not break the layout.
func EscapePlainField(value string) string {
	var b strings.Builder
	b.Grow(len(value))

	for _, r := range value {
		appendPlainFieldRune(&b, r)
	}

	return b.String()
}

func appendPlainFieldRune(b *strings.Builder, r rune) {
	switch r {
	case '\\':
		b.WriteString(`\\`)
		return
	case '\t':
		b.WriteString(`\t`)
		return
	case '\n':
		b.WriteString(`\n`)
		return
	case '\r':
		b.WriteString(`\r`)
		return
	}

	if r < 0x20 || r == 0x7f {
		writeHexEscape(b, r)
		return
	}
	if unicode.IsControl(r) {
		writeUnicodeEscape(b, r)
		return
	}

	b.WriteRune(r)
}

func writeHexEscape(b *strings.Builder, r rune) {
	const digits = "0123456789abcdef"

	b.WriteString(`\x`)
	b.WriteByte(digits[(r>>4)&0xf])
	b.WriteByte(digits[r&0xf])
}

func writeUnicodeEscape(b *strings.Builder, r rune) {
	const digits = "0123456789abcdef"

	if r <= 0xffff {
		b.WriteString(`\u`)
		for shift := 12; shift >= 0; shift -= 4 {
			b.WriteByte(digits[(r>>shift)&0xf])
		}
		return
	}

	b.WriteString(`\U`)
	for shift := 28; shift >= 0; shift -= 4 {
		b.WriteByte(digits[(r>>shift)&0xf])
	}
}
