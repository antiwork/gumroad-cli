package prompt

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func withConfirmInput(t *testing.T, input string, interactive bool) *os.File {
	t.Helper()

	oldIsTerminal := isTerminal
	isTerminal = func(int) bool { return interactive }
	t.Cleanup(func() { isTerminal = oldIsTerminal })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	go func() {
		_, _ = w.Write([]byte(input))
		_ = w.Close()
	}()

	return r
}

func TestConfirm_YesFlag(t *testing.T) {
	ok, err := Confirm("delete?", os.Stdin, &bytes.Buffer{}, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true when yes=true")
	}
}

func TestConfirm_YesTakesPrecedenceOverNoInput(t *testing.T) {
	ok, err := Confirm("delete?", os.Stdin, &bytes.Buffer{}, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true when yes=true even with noInput=true")
	}
}

func TestConfirm_NoInputWithoutYes(t *testing.T) {
	ok, err := Confirm("delete?", os.Stdin, &bytes.Buffer{}, false, true)
	if err == nil {
		t.Fatal("expected error when noInput=true and yes=false")
	}
	if ok {
		t.Error("expected false")
	}
	if !errors.Is(err, ErrConfirmationNoInput) {
		t.Fatalf("expected ErrConfirmationNoInput, got %v", err)
	}
	if err.Error() != "confirmation required but interactive prompts are disabled. Use --yes to skip confirmation" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestConfirm_NonInteractiveWithoutYes(t *testing.T) {
	ok, err := Confirm("delete?", withConfirmInput(t, "", false), &bytes.Buffer{}, false, false)
	if err == nil {
		t.Fatal("expected error when stdin is not interactive")
	}
	if ok {
		t.Error("expected false")
	}
	if !errors.Is(err, ErrConfirmationNonInteractive) {
		t.Fatalf("expected ErrConfirmationNonInteractive, got %v", err)
	}
	if err.Error() != "confirmation required but stdin is not interactive. Use --yes to skip confirmation" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestConfirm_NonFileReaderWithoutYes(t *testing.T) {
	ok, err := Confirm("delete?", strings.NewReader("yes\n"), &bytes.Buffer{}, false, false)
	if err == nil {
		t.Fatal("expected error when stdin is not a terminal-backed file")
	}
	if ok {
		t.Error("expected false")
	}
	if !errors.Is(err, ErrConfirmationNonInteractive) {
		t.Fatalf("expected ErrConfirmationNonInteractive, got %v", err)
	}
}

func TestConfirm_InteractiveYes(t *testing.T) {
	ok, err := Confirm("delete?", withConfirmInput(t, "y\n", true), &bytes.Buffer{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true for 'y' input")
	}
}

func TestConfirm_InteractiveNo(t *testing.T) {
	ok, err := Confirm("delete?", withConfirmInput(t, "n\n", true), &bytes.Buffer{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for 'n' input")
	}
}

func TestConfirm_InteractiveEmpty(t *testing.T) {
	ok, err := Confirm("delete?", withConfirmInput(t, "\n", true), &bytes.Buffer{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for empty input (default is No)")
	}
}

func TestConfirm_InteractiveYesFullWord(t *testing.T) {
	ok, err := Confirm("delete?", withConfirmInput(t, "yes\n", true), &bytes.Buffer{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true for 'yes' input")
	}
}

func TestConfirm_InteractiveYesWithoutNewline(t *testing.T) {
	ok, err := Confirm("delete?", withConfirmInput(t, "yes", true), &bytes.Buffer{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true for EOF-terminated 'yes' input")
	}
}

func TestConfirm_InteractiveEOF(t *testing.T) {
	_, err := Confirm("delete?", withConfirmInput(t, "", true), &bytes.Buffer{}, false, false)
	if err == nil {
		t.Fatal("expected error for EOF on stdin")
	}
}

func TestConfirm_DefaultsToProcessStreams(t *testing.T) {
	oldStdin := os.Stdin
	os.Stdin = withConfirmInput(t, "yes\n", true)
	defer func() { os.Stdin = oldStdin }()

	finish := captureProcessStderr(t)

	ok, err := Confirm("delete?", nil, nil, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}

	if got := finish(); !strings.Contains(got, "delete? [y/N] ") {
		t.Fatalf("expected prompt in stderr, got %q", got)
	}
}
