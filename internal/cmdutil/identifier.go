package cmdutil

import (
	"io"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/output"
)

func WriteIdentifierLine(w io.Writer, label, message, identifier string) error {
	if identifier == "" || strings.Contains(message, identifier) {
		return nil
	}
	return output.Writef(w, "%s: %s\n", label, identifier)
}
