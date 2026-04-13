package prompt

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSelect_ValidChoice(t *testing.T) {
	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	options := []SelectOption{
		{Label: "Option A", Value: "a"},
		{Label: "Option B", Value: "b"},
		{Label: "Option C", Value: "c"},
	}

	in, out := pipeWithInput(t, "2\n")
	var stderr bytes.Buffer
	value, err := Select("Pick one:", options, in, &stderr, false)
	_ = out
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "b" {
		t.Errorf("got %q, want %q", value, "b")
	}

	rendered := stderr.String()
	if !strings.Contains(rendered, "Pick one:") {
		t.Errorf("expected prompt message, got %q", rendered)
	}
	if !strings.Contains(rendered, "1) Option A") {
		t.Errorf("expected option 1, got %q", rendered)
	}
	if !strings.Contains(rendered, "Choose [1-3]:") {
		t.Errorf("expected choice prompt, got %q", rendered)
	}
}

func TestSelect_FirstOption(t *testing.T) {
	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	options := []SelectOption{
		{Label: "Only", Value: "only"},
	}

	in, out := pipeWithInput(t, "1\n")
	_ = out
	value, err := Select("Pick:", options, in, &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "only" {
		t.Errorf("got %q, want %q", value, "only")
	}
}

func TestSelect_InvalidChoiceTooHigh(t *testing.T) {
	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	options := []SelectOption{
		{Label: "A", Value: "a"},
		{Label: "B", Value: "b"},
	}

	in, out := pipeWithInput(t, "3\n")
	_ = out
	_, err := Select("Pick:", options, in, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected error for out-of-range choice")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("expected 'invalid choice' error, got %q", err.Error())
	}
}

func TestSelect_InvalidChoiceZero(t *testing.T) {
	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	options := []SelectOption{{Label: "A", Value: "a"}}

	in, out := pipeWithInput(t, "0\n")
	_ = out
	_, err := Select("Pick:", options, in, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected error for zero choice")
	}
}

func TestSelect_InvalidChoiceText(t *testing.T) {
	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	options := []SelectOption{{Label: "A", Value: "a"}}

	in, out := pipeWithInput(t, "abc\n")
	_ = out
	_, err := Select("Pick:", options, in, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected error for non-numeric choice")
	}
	if !strings.Contains(err.Error(), "invalid choice: abc") {
		t.Errorf("expected 'invalid choice: abc', got %q", err.Error())
	}
}

func TestSelect_NoInput(t *testing.T) {
	options := []SelectOption{{Label: "A", Value: "a"}}
	_, err := Select("Pick:", options, nil, &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("expected error with --no-input")
	}
	if !strings.Contains(err.Error(), "selection required but --no-input is set") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSelect_NonInteractive(t *testing.T) {
	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return false }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	options := []SelectOption{{Label: "A", Value: "a"}}
	in, out := pipeWithInput(t, "1\n")
	_ = out
	_, err := Select("Pick:", options, in, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected error for non-interactive stdin")
	}
	if !strings.Contains(err.Error(), "stdin is not interactive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSelect_EmptyOptions(t *testing.T) {
	_, err := Select("Pick:", nil, nil, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected error for empty options")
	}
	if !strings.Contains(err.Error(), "no options") {
		t.Errorf("unexpected error: %v", err)
	}
}

// pipeWithInput creates an os.File pair where the read end has the given content.
func pipeWithInput(t *testing.T, input string) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	_, err = w.WriteString(input)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()
	return r, w
}
