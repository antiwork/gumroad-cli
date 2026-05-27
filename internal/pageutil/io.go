package pageutil

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

const DefaultPath = "landing.html"

var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\)|[@-_])`)

func ReadHTML(opts cmdutil.Options, path string) (string, string, error) {
	if path == "" {
		path = DefaultPath
	}
	if path == "-" {
		data, err := io.ReadAll(opts.In())
		if err != nil {
			return "", "", fmt.Errorf("could not read stdin: %w", err)
		}
		return string(data), "stdin", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("could not read %s: %w", path, err)
	}
	return string(data), path, nil
}

func StripTerminalControls(value string) string {
	value = ansiPattern.ReplaceAllString(value, "")

	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == 0x7f || unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
