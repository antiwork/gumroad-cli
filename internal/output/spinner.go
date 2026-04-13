package output

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var isSpinnerTerminal = func(w io.Writer) bool {
	return isTerminalWriter(w)
}

type Spinner struct {
	message string
	writer  io.Writer
	done    chan struct{}
	mu      sync.Mutex
	wg      sync.WaitGroup
	active  bool
}

func NewSpinner(message string) *Spinner {
	return NewSpinnerTo(message, os.Stderr)
}

func NewSpinnerTo(message string, w io.Writer) *Spinner {
	if w == nil {
		w = os.Stderr
	}
	return &Spinner{
		message: message,
		writer:  w,
		done:    make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	// Only show spinner on an interactive terminal-backed writer.
	if !isSpinnerTerminal(s.writer) || isDumbTerminal() {
		return
	}

	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		i := 0
		for {
			select {
			case <-s.done:
				fmt.Fprintf(s.writer, "\r\033[K")
				return
			default:
				fmt.Fprintf(s.writer, "\r%s %s", frames[i%len(frames)], s.message)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	wasActive := s.active
	if s.active {
		close(s.done)
		s.active = false
	}
	s.mu.Unlock()
	if wasActive {
		s.wg.Wait()
	}
}
