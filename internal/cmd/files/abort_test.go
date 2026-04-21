package files

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestAbort_HappyPath_CallsAbortEndpoint(t *testing.T) {
	called := false
	var body map[string]string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/abort" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "bad", http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		body = map[string]string{
			"upload_id": r.PostForm.Get("upload_id"),
			"key":       r.PostForm.Get("key"),
		}
		called = true
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newAbortCmd(), testutil.JSONOutput(), testutil.Yes(true))
	cmd.SetArgs([]string{"--upload-id", "up-1", "--key", "attachments/u/k/original/file.bin"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !called {
		t.Fatal("abort endpoint was not called")
	}
	if body["upload_id"] != "up-1" || body["key"] != "attachments/u/k/original/file.bin" {
		t.Errorf("body = %+v", body)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true, got %v", resp)
	}
}

func TestAbort_NoInput_WithoutYes_BlocksBeforeAPICall(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("abort must not reach the API without confirmation")
		http.Error(w, "must not call", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newAbortCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--upload-id", "up-1", "--key", "attachments/u/k/original/file.bin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error in --no-input mode")
	}
}

func TestAbort_MissingUploadID_Errors(t *testing.T) {
	cmd := testutil.Command(newAbortCmd())
	cmd.SetArgs([]string{"--key", "attachments/u/k/original/file.bin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error for missing --upload-id")
	}
	if !strings.Contains(err.Error(), "--upload-id") {
		t.Errorf("error = %v, want mention of --upload-id", err)
	}
}

func TestAbort_MissingKey_Errors(t *testing.T) {
	cmd := testutil.Command(newAbortCmd())
	cmd.SetArgs([]string{"--upload-id", "up-1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error for missing --key")
	}
	if !strings.Contains(err.Error(), "--key") {
		t.Errorf("error = %v, want mention of --key", err)
	}
}

func TestAbort_StrayPositionalArg_RejectedBeforeAPICall(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("abort must not reach the API when extra args are present")
		http.Error(w, "must not call", http.StatusInternalServerError)
	})

	cmd := testutil.Command(newAbortCmd())
	cmd.SetArgs([]string{"--upload-id", "up-1", "--key", "attachments/u/k/original/p.bin", "stray"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error for stray positional arg")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Errorf("error = %v, want 'unexpected argument'", err)
	}
}

func TestFilesCmd_AbortIsRegistered(t *testing.T) {
	cmd := NewFilesCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "abort" {
			found = true
			break
		}
	}
	if !found {
		t.Error("abort subcommand not registered")
	}
}
