//go:build unix

package files

import (
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestComplete_RejectsFIFO(t *testing.T) {
	// os.Open on a FIFO blocks forever waiting for a writer. Without a
	// stat-before-open guard, pointing --recovery at a named pipe would
	// hang the recovery command and leave a multipart upload unreconciled.
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not reach API for a FIFO manifest path")
	})
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
		cmd.SetArgs([]string{"--recovery", path})
		done <- cmd.Execute()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for FIFO recovery path")
		}
		if !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("error = %v, want mention of non-regular file", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("files complete hung on FIFO recovery path")
	}
}
