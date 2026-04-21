package output

import (
	"io"
	"testing"
	"time"
)

func TestSpinner_NonTTY_DoesNotActivate(t *testing.T) {
	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return false }
	defer func() { isSpinnerTerminal = orig }()

	s := NewSpinner("loading")
	s.Start()
	defer s.Stop()

	s.mu.Lock()
	active := s.active
	s.mu.Unlock()

	if active {
		t.Error("spinner should not be active on non-TTY")
	}
}

func TestSpinner_TTY_ActivatesAndStops(t *testing.T) {
	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return true }
	defer func() { isSpinnerTerminal = orig }()

	s := NewSpinner("loading")
	s.Start()

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if !active {
		t.Error("spinner should be active on TTY after Start")
	}

	s.Stop()

	s.mu.Lock()
	active = s.active
	s.mu.Unlock()
	if active {
		t.Error("spinner should not be active after Stop")
	}
}

func TestSpinner_DoubleStop(t *testing.T) {
	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return true }
	defer func() { isSpinnerTerminal = orig }()

	s := NewSpinner("loading")
	s.Start()
	time.Sleep(10 * time.Millisecond)

	s.Stop()
	// Second stop should not panic
	s.Stop()
}

func TestSpinner_StopWithoutStart(t *testing.T) {
	s := NewSpinner("loading")
	// Should not panic
	s.Stop()
}

func TestSpinner_TermDumb_DoesNotActivate(t *testing.T) {
	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return true }
	defer func() { isSpinnerTerminal = orig }()

	t.Setenv("TERM", "dumb")

	s := NewSpinner("loading")
	s.Start()
	defer s.Stop()

	s.mu.Lock()
	active := s.active
	s.mu.Unlock()

	if active {
		t.Error("spinner should not be active when TERM=dumb")
	}
}

func TestSpinner_TermDumb_UsesCachedValue(t *testing.T) {
	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return true }
	defer func() { isSpinnerTerminal = orig }()

	t.Setenv("TERM", "dumb")

	s := NewSpinner("test")
	s.Start()
	defer s.Stop()

	s.mu.Lock()
	active := s.active
	s.mu.Unlock()

	if active {
		t.Error("spinner activated despite TERM=dumb")
	}
}

func TestNewSpinner(t *testing.T) {
	s := NewSpinner("test message")
	if s.message != "test message" {
		t.Errorf("got message=%q, want %q", s.message, "test message")
	}
	if s.done == nil {
		t.Error("done channel should not be nil")
	}
	if s.active {
		t.Error("spinner should not be active by default")
	}
}

func TestSpinner_SetMessage_UpdatesField(t *testing.T) {
	s := NewSpinner("initial")
	s.SetMessage("updated")

	s.mu.Lock()
	got := s.message
	s.mu.Unlock()

	if got != "updated" {
		t.Errorf("message = %q, want %q", got, "updated")
	}
}

func TestSpinner_SetMessage_BeforeStartAndAfterStop(t *testing.T) {
	s := NewSpinner("initial")
	// Before Start: field updates but no rendering goroutine exists.
	s.SetMessage("before-start")

	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return true }
	defer func() { isSpinnerTerminal = orig }()
	s.Start()
	time.Sleep(10 * time.Millisecond)
	s.Stop()

	// After Stop: SetMessage must not panic or race.
	s.SetMessage("after-stop")

	s.mu.Lock()
	got := s.message
	s.mu.Unlock()
	if got != "after-stop" {
		t.Errorf("message = %q, want %q", got, "after-stop")
	}
}

func TestSpinner_SetMessage_ConcurrentRenders(t *testing.T) {
	orig := isSpinnerTerminal
	isSpinnerTerminal = func(io.Writer) bool { return true }
	defer func() { isSpinnerTerminal = orig }()

	s := NewSpinnerTo("start", io.Discard)
	s.Start()
	defer s.Stop()

	// Race detector catches unsynchronized access against the
	// render goroutine that reads s.message on every tick.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			s.SetMessage("msg")
		}
		close(done)
	}()
	<-done
}
