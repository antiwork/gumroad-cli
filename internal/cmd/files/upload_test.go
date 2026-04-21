package files

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/antiwork/gumroad-cli/internal/upload"
)

// uploadServers wires a Rails mock (via testutil.Setup) plus an in-process
// TLS server for S3 part PUTs. The TLS server lets presigned URLs pass the
// upload package's https-only check without exposing a test hook from that
// package. The s3HTTPClientForTesting var is swapped in so S3 requests trust
// the self-signed cert.
type uploadServers struct {
	s3            *httptest.Server
	s3Calls       atomic.Int32
	completeCalls atomic.Int32
	presignBody   map[string]string

	presignHandler http.HandlerFunc
}

func newUploadServers(t *testing.T) *uploadServers {
	t.Helper()
	u := &uploadServers{
		presignBody: map[string]string{},
	}
	u.s3 = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.s3Calls.Add(1)
		if r.Method != http.MethodPut {
			t.Errorf("S3 got %s, want PUT", r.Method)
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(u.s3.Close)

	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = u.s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

	return u
}

// dispatch mounts the Rails handler as the testutil.Setup handler.
func (u *uploadServers) dispatch(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/files/presign":
			for k := range r.PostForm {
				u.presignBody[k] = r.PostForm.Get(k)
			}
			if u.presignHandler != nil {
				u.presignHandler(w, r)
				return
			}
			u.writeDefaultPresign(t, w)
		case "/files/complete":
			u.completeCalls.Add(1)
			testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/attachments/u/k/original/fixture.bin"})
		case "/files/abort":
			testutil.JSON(t, w, map[string]any{})
		default:
			t.Errorf("unexpected Rails path: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

func (u *uploadServers) writeDefaultPresign(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	testutil.JSON(t, w, map[string]any{
		"upload_id": "up-1",
		"key":       "attachments/u/k/original/fixture.bin",
		"file_url":  "https://example.com/attachments/u/k/original/fixture.bin",
		"parts": []map[string]any{
			{"part_number": 1, "presigned_url": u.s3.URL + "/part/1"},
		},
	})
}

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestUpload_HappyPath_PrintsURL(t *testing.T) {
	srv := newUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "hello")
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	got := strings.TrimSpace(out)
	want := "https://example.com/attachments/u/k/original/fixture.bin"
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if srv.s3Calls.Load() != 1 {
		t.Errorf("S3 calls = %d, want 1", srv.s3Calls.Load())
	}
	if srv.completeCalls.Load() != 1 {
		t.Errorf("complete calls = %d, want 1", srv.completeCalls.Load())
	}
	if srv.presignBody["filename"] != filepath.Base(path) {
		t.Errorf("presign filename = %q, want %q", srv.presignBody["filename"], filepath.Base(path))
	}
	if srv.presignBody["file_size"] != "5" {
		t.Errorf("presign file_size = %q, want 5", srv.presignBody["file_size"])
	}
}

func TestUpload_NameFlag_OverridesFilename(t *testing.T) {
	srv := newUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "payload")
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path, "--name", "Custom Name.zip"})

	_ = testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if got := srv.presignBody["filename"]; got != "Custom Name.zip" {
		t.Errorf("presign filename = %q, want %q", got, "Custom Name.zip")
	}
}

func TestUpload_JSONOutput_ReturnsFileURLField(t *testing.T) {
	srv := newUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "body")
	cmd := testutil.Command(newUploadCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]string
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if got := resp["file_url"]; got != "https://example.com/attachments/u/k/original/fixture.bin" {
		t.Errorf("file_url = %q", got)
	}
}

func TestUpload_JQExpression_ExtractsField(t *testing.T) {
	srv := newUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "body")
	cmd := testutil.Command(newUploadCmd(), testutil.JQ(".file_url"))
	cmd.SetArgs([]string{path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	got := strings.TrimSpace(out)
	got = strings.Trim(got, "\"")
	if got != "https://example.com/attachments/u/k/original/fixture.bin" {
		t.Errorf("jq output = %q", out)
	}
}

func TestUpload_PlainOutput_SingleRow(t *testing.T) {
	srv := newUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "body")
	cmd := testutil.Command(newUploadCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	got := strings.TrimSpace(out)
	if got != "https://example.com/attachments/u/k/original/fixture.bin" {
		t.Errorf("plain output = %q", out)
	}
}

func TestUpload_DryRun_NoNetworkAndPrintsPlan(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("Rails must not be called in --dry-run")
	})
	// Intentionally do not call newUploadServers: neither Rails nor S3 may
	// be contacted in dry-run mode. An un-injected s3HTTPClientForTesting
	// pointing at the production default would surface any S3 PUT attempt
	// as a connect failure, but we assert the cleaner invariant: we never
	// even get that far.

	path := writeFixture(t, "the bytes")
	cmd := testutil.Command(newUploadCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{path, "--name", "Gift.zip"})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "Dry run") {
		t.Errorf("missing dry run label: %q", out)
	}
	if !strings.Contains(out, "Gift.zip") {
		t.Errorf("missing overridden filename: %q", out)
	}
	if !strings.Contains(out, path) {
		t.Errorf("missing path: %q", out)
	}
}

func TestUpload_DryRunJSON_ReturnsPlanStructure(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("Rails must not be called in --dry-run")
	})

	path := writeFixture(t, "9 bytes!!")
	cmd := testutil.Command(newUploadCmd(), testutil.JSONOutput(), testutil.DryRun(true))
	cmd.SetArgs([]string{path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var plan dryRunUploadPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if !plan.DryRun || plan.Action != "upload" {
		t.Errorf("bad envelope: %+v", plan)
	}
	if plan.Path != path {
		t.Errorf("path = %q, want %q", plan.Path, path)
	}
	if plan.Filename != filepath.Base(path) {
		t.Errorf("filename = %q, want %q", plan.Filename, filepath.Base(path))
	}
	if plan.Size != 9 {
		t.Errorf("size = %d, want 9", plan.Size)
	}
	if plan.PartCount != 1 {
		t.Errorf("part_count = %d, want 1", plan.PartCount)
	}
}

func TestUpload_MissingFile_Errors(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("should not reach Rails when file is missing")
	})

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "does-not-exist")})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUpload_EmptyFile_Errors(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Error("should not reach Rails when file is empty")
	})

	path := writeFixture(t, "")
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want mention of empty", err)
	}
}

func TestUpload_CompleteAmbiguous_SurfacesUnknownStateError(t *testing.T) {
	srv := newUploadServers(t)
	// Force /files/complete to respond with an HTTP 502 so upload.Upload
	// treats the outcome as ambiguous and returns *upload.UnknownStateError
	// carrying the recovery handles.
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch r.URL.Path {
		case "/files/presign":
			srv.writeDefaultPresign(t, w)
		case "/files/complete":
			http.Error(w, `{"success":false,"message":"complete failed"}`, http.StatusBadGateway)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})
	// Also wire the S3 PUT so the orchestrator reaches the complete call.
	path := writeFixture(t, "payload")
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when /files/complete is ambiguous")
	}
	var state *upload.UnknownStateError
	if !errors.As(err, &state) {
		t.Fatalf("expected *upload.UnknownStateError, got %T: %v", err, err)
	}
	if state.UploadID == "" || state.Key == "" {
		t.Errorf("recovery handles empty: %+v", state)
	}
}

func TestUpload_PresignError_Propagates(t *testing.T) {
	srv := newUploadServers(t)
	srv.presignHandler = func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "permission denied",
		})
	}
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "body")
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected presign error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v, want contains 'permission denied'", err)
	}
}

func TestFilesCmd_UploadIsRegistered(t *testing.T) {
	cmd := NewFilesCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "upload" {
			found = true
			break
		}
	}
	if !found {
		t.Error("upload subcommand not registered")
	}
}

func TestUpload_NonQuietPath_StillPrintsURL(t *testing.T) {
	srv := newUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeFixture(t, "body")
	// Quiet(false) engages the spinner branch in runUpload. Routing Stderr
	// at io.Discard makes the terminal probe report "no TTY", so Start is
	// a no-op but the surrounding wiring (NewSpinnerTo + progress closure
	// with SetMessage) still runs, giving that branch coverage.
	cmd := testutil.Command(newUploadCmd(),
		testutil.Quiet(false),
		testutil.Stderr(io.Discard),
	)
	cmd.SetArgs([]string{path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "https://example.com/attachments/u/k/original/fixture.bin") {
		t.Errorf("stdout = %q", out)
	}
}

func TestRenderDryRun_PlainOutputRow(t *testing.T) {
	var buf bytes.Buffer
	opts := cmdutil.DefaultOptions()
	opts.Stdout = &buf
	opts.PlainOutput = true

	plan := upload.Plan{
		Path:      "/tmp/fixture.bin",
		Filename:  "fixture.bin",
		Size:      42,
		PartSize:  10,
		PartCount: 5,
	}
	if err := renderDryRun(opts, plan); err != nil {
		t.Fatalf("renderDryRun: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := "upload\t/tmp/fixture.bin\tfixture.bin\t42\t5"
	if got != want {
		t.Errorf("plain dry run = %q, want %q", got, want)
	}
}

func TestRenderDryRun_MultiPartPluralization(t *testing.T) {
	var buf bytes.Buffer
	opts := cmdutil.DefaultOptions()
	opts.Stdout = &buf

	plan := upload.Plan{
		Path:      "/tmp/big.bin",
		Filename:  "big.bin",
		Size:      500,
		PartSize:  100,
		PartCount: 5,
	}
	if err := renderDryRun(opts, plan); err != nil {
		t.Fatalf("renderDryRun: %v", err)
	}
	if !strings.Contains(buf.String(), "5 parts") {
		t.Errorf("expected '5 parts', got %q", buf.String())
	}
}

func TestSpinnerStatus_IncludesFilenameAndProgress(t *testing.T) {
	got := spinnerStatus("pack.zip", 1024*1024*4, "12.0 MB")
	if !strings.Contains(got, "pack.zip") {
		t.Errorf("spinnerStatus missing filename: %q", got)
	}
	if !strings.Contains(got, "4.0 MB") || !strings.Contains(got, "12.0 MB") {
		t.Errorf("spinnerStatus missing bytes: %q", got)
	}
}

func TestHumanBytes_Magnitudes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1500, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{12 * 1024 * 1024, "12.0 MB"},
		{2 * 1024 * 1024 * 1024, "2.0 GB"},
		// Stays at TB for very large values (prevents a stray "PB" label).
		{1024 * 1024 * 1024 * 1024 * 2048, "2048.0 TB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
