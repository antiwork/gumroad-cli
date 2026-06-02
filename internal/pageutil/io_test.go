package pageutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadHTMLDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	body := "<section>Buy</section>"
	if err := os.WriteFile(filepath.Join(dir, "landing.html"), []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	input, err := ReadHTML(bytes.NewBuffer(nil), "")
	if err != nil {
		t.Fatalf("ReadHTML returned error: %v", err)
	}
	if input.Source != DefaultHTMLPath {
		t.Errorf("got source %q, want %q", input.Source, DefaultHTMLPath)
	}
	if input.HTML != body {
		t.Errorf("got HTML %q, want %q", input.HTML, body)
	}
}

func TestReadHTMLStdin(t *testing.T) {
	input, err := ReadHTML(bytes.NewBufferString("<h1>stdin</h1>"), "-")
	if err != nil {
		t.Fatalf("ReadHTML returned error: %v", err)
	}
	if input.Source != "stdin" {
		t.Errorf("got source %q, want stdin", input.Source)
	}
	if input.HTML != "<h1>stdin</h1>" {
		t.Errorf("got HTML %q", input.HTML)
	}
}

func TestStripTerminalControls(t *testing.T) {
	got := StripTerminalControls("\x1b[31m<script>\x1b[0m\nbad\x00\x1b]0;title\a")
	if got != "<script>bad" {
		t.Errorf("got %q, want %q", got, "<script>bad")
	}
}
