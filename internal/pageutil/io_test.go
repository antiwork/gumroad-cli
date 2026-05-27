package pageutil

import (
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestReadHTMLReadsStdinDash(t *testing.T) {
	opts := testutil.TestOptions(testutil.Stdin(strings.NewReader("<main>stdin</main>")))

	got, source, err := ReadHTML(opts, "-")
	if err != nil {
		t.Fatalf("ReadHTML failed: %v", err)
	}
	if got != "<main>stdin</main>" {
		t.Fatalf("got %q", got)
	}
	if source != "stdin" {
		t.Fatalf("source = %q", source)
	}
}

func TestStripTerminalControlsRemovesANSIAndControlRunes(t *testing.T) {
	got := StripTerminalControls("ok\x1b[31mred\x1b[0m\nbad\x00")
	if got != "okredbad" {
		t.Fatalf("got %q", got)
	}
}
