package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/antiwork/gumroad-cli/internal/upload"
)

func writeRecovery(t *testing.T, data any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recovery.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create recovery: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := json.NewEncoder(f).Encode(data); err != nil {
		t.Fatalf("encode recovery: %v", err)
	}
	return path
}

func TestComplete_HappyPath_IndexedPartsAndReturnsFileURL(t *testing.T) {
	var received map[string]string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/complete" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "bad", http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		received = map[string]string{}
		for k, v := range r.PostForm {
			received[k] = strings.Join(v, ",")
		}
		testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/final"})
	})

	recovery := map[string]any{
		"upload_id": "up-9",
		"key":       "attachments/u/k/original/p.bin",
		"completed_parts": []map[string]any{
			{"part_number": 1, "etag": "etag-1"},
			{"part_number": 2, "etag": "etag-2"},
		},
	}
	recoveryPath := writeRecovery(t, recovery)

	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if received["upload_id"] != "up-9" {
		t.Errorf("upload_id = %q", received["upload_id"])
	}
	if received["key"] != "attachments/u/k/original/p.bin" {
		t.Errorf("key = %q", received["key"])
	}
	for i := 0; i < 2; i++ {
		if got := received[fmt.Sprintf("parts[%d][part_number]", i)]; got == "" {
			t.Errorf("missing parts[%d][part_number]", i)
		}
		if got := received[fmt.Sprintf("parts[%d][etag]", i)]; got == "" {
			t.Errorf("missing parts[%d][etag]", i)
		}
	}
	if got := strings.TrimSpace(out); got != "https://example.com/final" {
		t.Errorf("expected canonical file_url on stdout, got %q", got)
	}
}

func TestComplete_JSON_EmitsFileURLEnvelope(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/final"})
	})

	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-j",
		"key":             "k",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.JSONOutput(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("expected JSON output, got %v: %s", err, out)
	}
	if got["file_url"] != "https://example.com/final" {
		t.Errorf("file_url = %q", got["file_url"])
	}
}

func TestComplete_ManifestWithoutFileURL_NoInput_StillBlocksReplay(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not replay without confirmation even when file_url is absent")
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-no-url",
		"key":             "k",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error even when manifest has no file_url")
	}
}

func TestComplete_ManifestWithFileURL_NoInput_BlocksReplay(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not replay /files/complete without confirmation when file_url is set")
	})

	recoveryPath := writeRecovery(t, map[string]any{
		"file_url":        "https://example.com/maybe-committed",
		"upload_id":       "up-maybe",
		"key":             "attachments/u/k/original/p.bin",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})

	cmd := testutil.Command(newCompleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error when file_url is present and --no-input is set")
	}
}

func TestComplete_ManifestWithFileURL_YesFlag_AllowsReplay(t *testing.T) {
	called := false
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/final"})
	})

	recoveryPath := writeRecovery(t, map[string]any{
		"file_url":        "https://example.com/maybe",
		"upload_id":       "up-ack",
		"key":             "k",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	_ = testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !called {
		t.Fatal("expected /files/complete to run when --yes acknowledges duplicate risk")
	}
}

func TestComplete_MissingRecovery_Errors(t *testing.T) {
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--recovery") {
		t.Fatalf("expected missing flag error, got %v", err)
	}
}

func TestComplete_EmptyCompletedParts_Errors(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not reach API when manifest is empty")
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-1",
		"key":             "k",
		"completed_parts": []any{},
	})
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no completed_parts") {
		t.Fatalf("expected empty parts error, got %v", err)
	}
}

func TestComplete_MissingEtag_Errors(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not reach API when manifest has a missing etag")
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id": "up-1",
		"key":       "k",
		"completed_parts": []map[string]any{
			{"part_number": 1, "etag": ""},
		},
	})
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "etag") {
		t.Fatalf("expected etag validation error, got %v", err)
	}
}

func TestComplete_StdinRecovery_WorksForPipedJQ(t *testing.T) {
	var received bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		received = true
		testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/final"})
	})

	recovery := `{"upload_id":"up-stdin","key":"k","completed_parts":[{"part_number":1,"etag":"e1"}]}`
	cmd := testutil.Command(newCompleteCmd(), testutil.Stdin(strings.NewReader(recovery)), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", "-"})
	_ = testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !received {
		t.Fatal("expected API to be called via stdin manifest")
	}
}

func TestComplete_DryRun_EmitsRequestPlan(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("dry-run must not reach the API")
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-dry",
		"key":             "k",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "POST") || !strings.Contains(out, "/files/complete") {
		t.Errorf("expected dry-run output, got %q", out)
	}
}

func TestComplete_AmbiguousReplayFailure_PreservesManifest(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"success":false,"message":"upstream timeout"}`, http.StatusBadGateway)
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-retry",
		"key":             "attachments/u/k/original/p.bin",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous finalize error")
	}
	var state *upload.UnknownStateError
	if !errors.As(err, &state) {
		t.Fatalf("expected *upload.UnknownStateError to preserve manifest, got %T: %v", err, err)
	}
	if state.UploadID != "up-retry" || state.Key != "attachments/u/k/original/p.bin" {
		t.Errorf("manifest not preserved: %+v", state)
	}
	if len(state.CompletedParts) != 1 || state.CompletedParts[0].ETag != "e1" {
		t.Errorf("completed_parts not preserved: %+v", state.CompletedParts)
	}
}

func TestComplete_DefinitiveFailure_PreservesFullManifestForRetry(t *testing.T) {
	// Real failure mode: stdin-piped recovery hits 4xx, user loses the
	// only copy of the manifest unless CompleteRejectedError carries
	// file_url + completed_parts through.
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/files/complete" {
			testutil.RawJSON(t, w, `{"success":false,"message":"token invalid"}`)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"file_url":  "https://example.com/attachments/u/k/file.bin",
		"upload_id": "up-retry",
		"key":       "attachments/u/k/original/file.bin",
		"completed_parts": []map[string]any{
			{"part_number": 1, "etag": "etag-one"},
			{"part_number": 2, "etag": "etag-two"},
		},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected api error")
	}
	var rejected *CompleteRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("expected *CompleteRejectedError, got %T: %v", err, err)
	}
	if rejected.FileURL == "" || rejected.UploadID == "" || rejected.Key == "" {
		t.Errorf("scalar handles not preserved: %+v", rejected)
	}
	if len(rejected.CompletedParts) != 2 || rejected.CompletedParts[0].ETag != "etag-one" {
		t.Errorf("completed_parts not preserved: %+v", rejected.CompletedParts)
	}
}

func TestComplete_DefinitiveAPIFailure_PreservesHandlesWithoutAutoAbort(t *testing.T) {
	// A 4xx rejection in this recovery command may be a recoverable caller
	// mistake (wrong token, fixable manifest). Auto-aborting would destroy
	// the same parts still live on S3; the command must preserve handles
	// and let the user choose retry vs. abort.
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/complete":
			testutil.RawJSON(t, w, `{"success":false,"message":"invalid etag"}`)
		case "/files/abort":
			t.Error("files complete must not auto-abort on definitive rejection")
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-final",
		"key":             "attachments/u/k/original/p.bin",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "bad"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected api error")
	}
	var state *upload.UnknownStateError
	if errors.As(err, &state) {
		t.Fatalf("definitive rejection must not be wrapped as UnknownStateError (got one: %+v)", state)
	}
	var rejected *CompleteRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("expected *CompleteRejectedError to carry orphan handles, got %T: %v", err, err)
	}
	if rejected.UploadID != "up-final" || rejected.Key != "attachments/u/k/original/p.bin" {
		t.Errorf("orphan handles lost: %+v", rejected)
	}
}

func TestComplete_ManifestNonContiguousParts_Errors(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not reach API with malformed part numbering")
	})
	// Uploader invariant: contiguous 1..N. A manifest with 1,3 or 2,1
	// would finalize the wrong bytes order on the server.
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id": "u",
		"key":       "k",
		"completed_parts": []map[string]any{
			{"part_number": 1, "etag": "a"},
			{"part_number": 3, "etag": "b"},
		},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "expected 2") {
		t.Fatalf("expected part-number validation error, got %v", err)
	}
}

func TestComplete_StdinRecoveryWithoutYes_Errors(t *testing.T) {
	// --recovery - consumes stdin for the manifest, so the duplicate-risk
	// prompt would deadlock. The command should reject this combination
	// with a clear message before any manifest is read.
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("must not reach API when confirmation has no input channel")
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Stdin(strings.NewReader(`{"upload_id":"u","key":"k","completed_parts":[{"part_number":1,"etag":"e"}]}`)))
	cmd.SetArgs([]string{"--recovery", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error for --recovery - without --yes")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should direct to --yes: %v", err)
	}
}

func TestComplete_APIResponseMissingFileURL_Errors(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})
	recoveryPath := writeRecovery(t, map[string]any{
		"upload_id":       "up-x",
		"key":             "k",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "file_url") {
		t.Fatalf("expected missing file_url error, got %v", err)
	}
}

func TestComplete_MissingUploadIDInManifest_Errors(t *testing.T) {
	recoveryPath := writeRecovery(t, map[string]any{
		"key":             "k",
		"completed_parts": []map[string]any{{"part_number": 1, "etag": "e1"}},
	})
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{"--recovery", recoveryPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "upload_id") {
		t.Fatalf("expected upload_id error, got %v", err)
	}
}

func TestComplete_BadRecoveryJSON_Errors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{"--recovery", path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestComplete_OversizedManifest_Errors(t *testing.T) {
	// A runaway producer piped into `--recovery -` (or a wrong large file)
	// must fail fast instead of hanging or exhausting memory.
	path := filepath.Join(t.TempDir(), "huge.json")
	// Write 3 MB of zeros — well above the 2 MB cap.
	big := make([]byte, 3*1024*1024)
	if err := os.WriteFile(path, big, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{"--recovery", path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size-cap error, got %v", err)
	}
}

func TestComplete_RecoveryFileMissing_Errors(t *testing.T) {
	cmd := testutil.Command(newCompleteCmd())
	cmd.SetArgs([]string{"--recovery", filepath.Join(t.TempDir(), "does-not-exist.json")})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestFilesCmd_CompleteIsRegistered(t *testing.T) {
	cmd := NewFilesCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "complete" {
			found = true
			break
		}
	}
	if !found {
		t.Error("complete subcommand not registered")
	}
}
