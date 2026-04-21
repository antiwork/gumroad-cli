package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

// TestMain bypasses the https-only destination check for the whole package
// because every test targets an httptest.Server on http://127.0.0.1. The
// check itself is exercised directly via TestValidatePresignParts_HTTPSOnly.
func TestMain(m *testing.M) {
	allowInsecureUploadDestination = true
	os.Exit(m.Run())
}

// setTestPartSize swaps activePartSize for the duration of a test so we can
// exercise multi-part flows without 100 MB+ fixtures.
func setTestPartSize(t *testing.T, v int64) {
	t.Helper()
	prev := activePartSize
	activePartSize = v
	t.Cleanup(func() { activePartSize = prev })
}

// shortBackoff collapses part-upload retry waits so retry tests don't spend
// ~600 ms in backoff sleeps.
func shortBackoff(t *testing.T) {
	t.Helper()
	prevInitial, prevMax := initialPartRetryBackoff, maxPartRetryBackoff
	initialPartRetryBackoff = time.Millisecond
	maxPartRetryBackoff = time.Millisecond
	t.Cleanup(func() {
		initialPartRetryBackoff = prevInitial
		maxPartRetryBackoff = prevMax
	})
}

// writeFile creates a fixture with `size` bytes of predictable content so
// tests can verify that per-part byte ranges were streamed correctly.
func writeFile(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 1024)
	for i := range buf {
		buf[i] = byte(i)
	}
	remaining := size
	for remaining > 0 {
		n := int64(len(buf))
		if n > remaining {
			n = remaining
		}
		if _, err := f.Write(buf[:n]); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		remaining -= n
	}
	return path
}

// railsMock tracks calls to the Rails endpoints and delegates each to a
// handler. Any handler left nil returns an unexpected-endpoint failure.
type railsMock struct {
	presign  http.HandlerFunc
	complete http.HandlerFunc
	abortH   http.HandlerFunc

	presignCalls  atomic.Int32
	completeCalls atomic.Int32
	abortCalls    atomic.Int32

	completeBody url.Values
	completeMu   sync.Mutex
}

// dispatch routes the request by path and records the call. It also captures
// the parsed form body from /files/complete because several tests assert on
// the indexed parts[n][...] encoding.
func (m *railsMock) dispatch(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch r.URL.Path {
	case "/files/presign":
		m.presignCalls.Add(1)
		if m.presign == nil {
			t.Errorf("unexpected /files/presign call")
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		m.presign(w, r)
	case "/files/complete":
		m.completeCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse complete form: %v", err)
		}
		m.completeMu.Lock()
		m.completeBody = r.PostForm
		m.completeMu.Unlock()
		if m.complete == nil {
			t.Errorf("unexpected /files/complete call")
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		m.complete(w, r)
	case "/files/abort":
		m.abortCalls.Add(1)
		if m.abortH == nil {
			testutil.JSON(t, w, map[string]any{})
			return
		}
		m.abortH(w, r)
	default:
		t.Errorf("unexpected Rails path: %s", r.URL.Path)
		http.Error(w, "unexpected", http.StatusNotFound)
	}
}

func (m *railsMock) completeForm() url.Values {
	m.completeMu.Lock()
	defer m.completeMu.Unlock()
	return m.completeBody
}

// setupServers wires a Rails mock (via testutil.Setup so the env + token are
// configured) and an S3 mock. The returned client hits the Rails mock; the
// S3 URL is available for the Rails mock to embed in presign responses.
func setupServers(t *testing.T, mock *railsMock, s3 http.Handler) (*api.Client, string) {
	t.Helper()
	s3srv := httptest.NewServer(s3)
	t.Cleanup(s3srv.Close)

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		mock.dispatch(t, w, r)
	})

	client := api.NewClient("test-token", "test", false)
	return client, s3srv.URL
}

func writePresignResponse(t *testing.T, w http.ResponseWriter, uploadID, key, fileURL string, partURLs []string) {
	t.Helper()
	parts := make([]map[string]any, len(partURLs))
	for i, u := range partURLs {
		parts[i] = map[string]any{
			"part_number":   i + 1,
			"presigned_url": u,
		}
	}
	testutil.JSON(t, w, map[string]any{
		"upload_id": uploadID,
		"key":       key,
		"file_url":  fileURL,
		"parts":     parts,
	})
}

func partPath(i int) string {
	return "/part/" + strconv.Itoa(i)
}

func partNumberFromPath(p string) int {
	parts := strings.Split(p, "/")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

// --- Describe tests ---

func TestDescribe_HappyPath(t *testing.T) {
	setTestPartSize(t, 10)
	path := writeFile(t, 25)

	plan, err := Describe(path, Options{})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if plan.Path != path {
		t.Errorf("Path = %q, want %q", plan.Path, path)
	}
	if plan.Filename != filepath.Base(path) {
		t.Errorf("Filename = %q, want %q", plan.Filename, filepath.Base(path))
	}
	if plan.Size != 25 {
		t.Errorf("Size = %d, want 25", plan.Size)
	}
	if plan.PartSize != 10 {
		t.Errorf("PartSize = %d, want 10", plan.PartSize)
	}
	if plan.PartCount != 3 {
		t.Errorf("PartCount = %d, want 3", plan.PartCount)
	}
}

func TestDescribe_FilenameOverride(t *testing.T) {
	setTestPartSize(t, 10)
	path := writeFile(t, 5)

	plan, err := Describe(path, Options{Filename: "pack.zip"})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if plan.Filename != "pack.zip" {
		t.Errorf("Filename = %q, want pack.zip", plan.Filename)
	}
}

func TestDescribe_RejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = f.Close()

	_, err = Describe(path, Options{})
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want mention of empty", err)
	}
}

func TestDescribe_RejectsOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Sparse file: reports 20 GB + 1 B but occupies ~0 bytes on disk.
	if err := f.Truncate(MaxFileSize + 1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	_ = f.Close()

	_, err = Describe(path, Options{})
	if err == nil {
		t.Fatal("expected error for oversize file")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want mention of exceeds", err)
	}
}

func TestDescribe_RejectsDirectory(t *testing.T) {
	_, err := Describe(t.TempDir(), Options{})
	if err == nil {
		t.Fatal("expected error for directory")
	}
}

func TestDescribe_RejectsMissing(t *testing.T) {
	_, err := Describe(filepath.Join(t.TempDir(), "does-not-exist"), Options{})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- Upload happy paths ---

func TestUpload_HappyPath_MultiPart(t *testing.T) {
	setTestPartSize(t, 5)
	const fileSize int64 = 12 // 3 parts: 5+5+2
	path := writeFile(t, fileSize)

	var s3mu sync.Mutex
	received := map[int][]byte{}
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("S3 got %s, want PUT", r.Method)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		pn := partNumberFromPath(r.URL.Path)
		s3mu.Lock()
		received[pn] = body
		s3mu.Unlock()
		w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, pn))
		w.WriteHeader(http.StatusOK)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)

	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse: %v", err)
		}
		if r.FormValue("filename") != filepath.Base(path) {
			t.Errorf("filename = %q", r.FormValue("filename"))
		}
		if r.FormValue("file_size") != strconv.FormatInt(fileSize, 10) {
			t.Errorf("file_size = %q", r.FormValue("file_size"))
		}
		urls := []string{s3URL + partPath(1), s3URL + partPath(2), s3URL + partPath(3)}
		writePresignResponse(t, w, "up-1", "attachments/user/abc/original/fixture.bin",
			"https://example.com/attachments/user/abc/original/fixture.bin", urls)
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"file_url": "https://example.com/attachments/user/abc/original/fixture.bin",
		})
	}

	fileURL, err := Upload(context.Background(), client, path, Options{})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if fileURL != "https://example.com/attachments/user/abc/original/fixture.bin" {
		t.Errorf("fileURL = %q", fileURL)
	}
	if mock.presignCalls.Load() != 1 {
		t.Errorf("presignCalls = %d, want 1", mock.presignCalls.Load())
	}
	if mock.completeCalls.Load() != 1 {
		t.Errorf("completeCalls = %d, want 1", mock.completeCalls.Load())
	}
	if mock.abortCalls.Load() != 0 {
		t.Errorf("abortCalls = %d, want 0 (no failure)", mock.abortCalls.Load())
	}

	// Verify bytes: part 1 = bytes 0..5, part 2 = 5..10, part 3 = 10..12.
	original, _ := os.ReadFile(path)
	s3mu.Lock()
	defer s3mu.Unlock()
	for i, want := range [][]byte{original[0:5], original[5:10], original[10:12]} {
		got := received[i+1]
		if !bytes.Equal(got, want) {
			t.Errorf("part %d bytes = %v, want %v", i+1, got, want)
		}
	}

	// Verify complete form has indexed parts[n][part_number]/[etag].
	form := mock.completeForm()
	if form.Get("upload_id") != "up-1" {
		t.Errorf("upload_id = %q", form.Get("upload_id"))
	}
	for i := 0; i < 3; i++ {
		if got := form.Get(fmt.Sprintf("parts[%d][part_number]", i)); got != strconv.Itoa(i+1) {
			t.Errorf("parts[%d][part_number] = %q", i, got)
		}
		if got := form.Get(fmt.Sprintf("parts[%d][etag]", i)); got != fmt.Sprintf("etag-%d", i+1) {
			t.Errorf("parts[%d][etag] = %q", i, got)
		}
	}
}

func TestUpload_HappyPath_SinglePart(t *testing.T) {
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"single-etag"`)
		w.WriteHeader(http.StatusOK)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)

	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "up-1", "attachments/u/key/original/fixture.bin",
			"https://example.com/u/fixture.bin", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/u/fixture.bin"})
	}

	fileURL, err := Upload(context.Background(), client, path, Options{})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if fileURL != "https://example.com/u/fixture.bin" {
		t.Errorf("fileURL = %q", fileURL)
	}

	form := mock.completeForm()
	if form.Get("parts[0][etag]") != "single-etag" {
		t.Errorf("parts[0][etag] = %q (ETag quotes not stripped?)", form.Get("parts[0][etag]"))
	}
}

func TestUpload_FilenameOverride(t *testing.T) {
	setTestPartSize(t, 100)
	path := writeFile(t, 10)

	var seenFilename string
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"e"`)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)

	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		seenFilename = r.FormValue("filename")
		writePresignResponse(t, w, "up", "k", "https://f", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://f"})
	}

	if _, err := Upload(context.Background(), client, path, Options{Filename: "override.zip"}); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if seenFilename != "override.zip" {
		t.Errorf("seenFilename = %q, want override.zip", seenFilename)
	}
}

func TestUpload_ProgressCallback(t *testing.T) {
	setTestPartSize(t, 5)
	const fileSize int64 = 12
	path := writeFile(t, fileSize)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		pn := partNumberFromPath(r.URL.Path)
		w.Header().Set("ETag", fmt.Sprintf(`"e-%d"`, pn))
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		urls := []string{s3URL + partPath(1), s3URL + partPath(2), s3URL + partPath(3)}
		writePresignResponse(t, w, "u", "k", "https://f", urls)
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://f"})
	}

	var last atomic.Int64
	var calls atomic.Int32
	_, err := Upload(context.Background(), client, path, Options{
		Concurrency: 1,
		Progress: func(n int64) {
			if prev := last.Load(); n < prev {
				t.Errorf("progress went backwards: %d < %d", n, prev)
			}
			last.Store(n)
			calls.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// Progress is async; poll for the expected 3 callbacks.
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := last.Load(); got != fileSize {
		t.Errorf("final progress = %d, want %d", got, fileSize)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("progress calls = %d, want 3 (one per part)", got)
	}
}

// --- Upload failure paths ---

func TestUpload_PresignFailure(t *testing.T) {
	setTestPartSize(t, 100)
	path := writeFile(t, 10)

	mock := &railsMock{}
	client, _ := setupServers(t, mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("S3 should not be called when presign fails")
	}))

	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"success":false,"message":"filename is required"}`)
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if mock.abortCalls.Load() != 0 {
		t.Errorf("abortCalls = %d, want 0 (no upload to abort)", mock.abortCalls.Load())
	}
	if mock.completeCalls.Load() != 0 {
		t.Errorf("completeCalls = %d, want 0", mock.completeCalls.Load())
	}
}

func TestUpload_PartFailureRetries(t *testing.T) {
	setTestPartSize(t, 100)
	shortBackoff(t)
	path := writeFile(t, 50)

	var attempts atomic.Int32
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"retried"`)
		w.WriteHeader(http.StatusOK)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://f"})
	}

	fileURL, err := Upload(context.Background(), client, path, Options{})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if fileURL != "https://f" {
		t.Errorf("fileURL = %q", fileURL)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
	if mock.abortCalls.Load() != 0 {
		t.Errorf("abortCalls = %d, want 0 (retry succeeded)", mock.abortCalls.Load())
	}
}

func TestUpload_PartFailureExhausts(t *testing.T) {
	setTestPartSize(t, 100)
	shortBackoff(t)
	path := writeFile(t, 50)

	var attempts atomic.Int32
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "still overloaded", http.StatusServiceUnavailable)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if attempts.Load() != int32(maxPartRetries+1) {
		t.Errorf("attempts = %d, want %d", attempts.Load(), maxPartRetries+1)
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1", mock.abortCalls.Load())
	}
	if mock.completeCalls.Load() != 0 {
		t.Errorf("completeCalls = %d, want 0", mock.completeCalls.Load())
	}
}

func TestUpload_CompleteFailure(t *testing.T) {
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"e"`)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"success":false,"message":"invalid key"}`)
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrCompleteStateUnknown) {
		t.Errorf("definitive 4xx should not be wrapped as ErrCompleteStateUnknown: %v", err)
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1", mock.abortCalls.Load())
	}
}

func TestUpload_CompleteAmbiguousResponse(t *testing.T) {
	// success:true but file_url missing: the server may or may not have
	// committed. Must surface as UnknownStateError (matching
	// ErrCompleteStateUnknown via errors.Is) with populated recovery
	// metadata, and must NOT abort — a blind abort could tear down a
	// successful commit.
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"e"`)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "up-7", "attachments/key-7", "https://example/u/7", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCompleteStateUnknown) {
		t.Errorf("errors.Is(err, ErrCompleteStateUnknown) = false, want true: %v", err)
	}
	var unk *UnknownStateError
	if !errors.As(err, &unk) {
		t.Fatalf("errors.As *UnknownStateError failed: %v", err)
	}
	if unk.FileURL != "https://example/u/7" {
		t.Errorf("FileURL = %q", unk.FileURL)
	}
	if unk.UploadID != "up-7" {
		t.Errorf("UploadID = %q", unk.UploadID)
	}
	if unk.Key != "attachments/key-7" {
		t.Errorf("Key = %q", unk.Key)
	}
	if unk.Cause == nil {
		t.Error("Cause is nil")
	}
	if len(unk.CompletedParts) != 1 {
		t.Fatalf("CompletedParts = %d, want 1", len(unk.CompletedParts))
	}
	if unk.CompletedParts[0].PartNumber != 1 || unk.CompletedParts[0].ETag != "e" {
		t.Errorf("CompletedParts[0] = %+v", unk.CompletedParts[0])
	}
	if mock.abortCalls.Load() != 0 {
		t.Errorf("abortCalls = %d, want 0 (ambiguous state, must not abort)", mock.abortCalls.Load())
	}
}

func TestUpload_CompleteSuccessFalseIsDefinitive(t *testing.T) {
	// Gumroad returns many rejections as HTTP 200 with {"success":false}.
	// api.Client surfaces that as an APIError with StatusCode==200. Upload
	// must treat it as a definitive rejection (abort, no ambiguous wrap).
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"e"`)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		// HTTP 200, but envelope says the request failed.
		_, _ = io.WriteString(w, `{"success":false,"message":"invalid etag for part 1"}`)
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrCompleteStateUnknown) {
		t.Errorf("explicit rejection must not be ErrCompleteStateUnknown: %v", err)
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1 (definitive rejection)", mock.abortCalls.Load())
	}
}

func TestUpload_CompleteTransient4xx(t *testing.T) {
	// 408/409/429 are 4xx but not definitive rejections — a 429 rate-limited
	// response, for instance, does not prove the commit did not happen.
	// Treat these as ambiguous and skip abort.
	for _, status := range []int{http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			setTestPartSize(t, 100)
			path := writeFile(t, 50)

			s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("ETag", `"e"`)
			})

			mock := &railsMock{}
			client, s3URL := setupServers(t, mock, s3Handler)
			mock.presign = func(w http.ResponseWriter, r *http.Request) {
				writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
			}
			mock.complete = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"success":false,"message":"transient %d"}`, status)
			}

			_, err := Upload(context.Background(), client, path, Options{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrCompleteStateUnknown) {
				t.Errorf("error = %v, want wrapped ErrCompleteStateUnknown", err)
			}
			if mock.abortCalls.Load() != 0 {
				t.Errorf("abortCalls = %d, want 0 (transient 4xx is ambiguous, must not abort)", mock.abortCalls.Load())
			}
		})
	}
}

func TestUpload_CompleteServerError(t *testing.T) {
	// /files/complete 5xx is ambiguous. Must wrap with ErrCompleteStateUnknown
	// and must NOT fire abort (could race the server's in-flight commit or
	// tear down a finalized upload).
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"e"`)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"success":false,"message":"database unreachable"}`)
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCompleteStateUnknown) {
		t.Errorf("error = %v, want wrapped ErrCompleteStateUnknown", err)
	}
	if mock.abortCalls.Load() != 0 {
		t.Errorf("abortCalls = %d, want 0 (5xx is ambiguous, must not abort)", mock.abortCalls.Load())
	}
}

func TestUpload_ProgressCallbackMonotonicUnderConcurrency(t *testing.T) {
	// With Concurrency > 1, parts finish in parallel. The serialized +
	// monotonic contract on Options.Progress must hold: no callback runs
	// concurrently, and cumulative bytes never go backwards.
	setTestPartSize(t, 5)
	const fileSize int64 = 20 // 4 parts of 5
	path := writeFile(t, fileSize)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		pn := partNumberFromPath(r.URL.Path)
		w.Header().Set("ETag", fmt.Sprintf(`"e-%d"`, pn))
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		urls := []string{s3URL + partPath(1), s3URL + partPath(2), s3URL + partPath(3), s3URL + partPath(4)}
		writePresignResponse(t, w, "u", "k", "https://f", urls)
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://f"})
	}

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	var last atomic.Int64
	var calls atomic.Int32

	_, err := Upload(context.Background(), client, path, Options{
		Concurrency: 4,
		Progress: func(n int64) {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				max := maxInFlight.Load()
				if cur <= max || maxInFlight.CompareAndSwap(max, cur) {
					break
				}
			}
			if prev := last.Load(); n < prev {
				t.Errorf("progress went backwards: %d after %d", n, prev)
			}
			last.Store(n)
			calls.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// Progress is async: poll for all 4 callbacks before asserting the final
	// value. Doc comment on Options.Progress explicitly allows callbacks to
	// run after Upload returns.
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := maxInFlight.Load(); got > 1 {
		t.Errorf("maxInFlight = %d, want 1 (callback must not run concurrently)", got)
	}
	if got := last.Load(); got != fileSize {
		t.Errorf("final progress = %d, want %d", got, fileSize)
	}
}

func TestUpload_AbortFailureSurfacesCleanupInfo(t *testing.T) {
	// When /files/abort itself fails after parts were already in flight, the
	// caller needs both the original error AND the upload_id/key to retry
	// cleanup later. Upload joins them so errors.Is still matches the cause
	// and errors.As surfaces *CleanupFailedError.
	setTestPartSize(t, 100)
	shortBackoff(t)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "still overloaded", http.StatusServiceUnavailable)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "up-99", "attachments/key-99", "https://f", []string{s3URL + partPath(1)})
	}
	mock.abortH = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"success":false,"message":"abort failed"}`)
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected part-upload error")
	}
	if !strings.Contains(err.Error(), "upload part 1") {
		t.Errorf("original part error not present in %v", err)
	}
	var cleanup *CleanupFailedError
	if !errors.As(err, &cleanup) {
		t.Fatalf("errors.As *CleanupFailedError failed: %v", err)
	}
	if cleanup.UploadID != "up-99" || cleanup.Key != "attachments/key-99" {
		t.Errorf("cleanup metadata = %+v", cleanup)
	}
	if cleanup.Cause == nil {
		t.Error("CleanupFailedError.Cause is nil")
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1", mock.abortCalls.Load())
	}
}

func TestUpload_ExpiredPresign(t *testing.T) {
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	var attempts atomic.Int32
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><Error><Code>AccessDenied</Code><Message>%s</Message></Error>`, s3ExpiredMarker)
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPresignExpired) {
		t.Errorf("error = %v, want ErrPresignExpired", err)
	}
	// Expired should NOT retry.
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (expiry must not retry)", attempts.Load())
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1", mock.abortCalls.Load())
	}
}

func TestUpload_ContextCancellation(t *testing.T) {
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("S3 should not be reached when context is canceled")
	})

	mock := &railsMock{}
	client, _ := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		t.Error("presign should not run when context is canceled before Upload")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Upload(ctx, client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestUpload_AbortRunsAfterCanceledPartUpload(t *testing.T) {
	// The plan explicitly calls out that context cancellation must still
	// trigger /files/abort. safeAbort uses a fresh background context with
	// abortTimeout precisely so a canceled upload does not leak a multipart
	// upload on S3.
	setTestPartSize(t, 100)
	shortBackoff(t)
	path := writeFile(t, 50)

	// release is closed on test cleanup so the S3 handler unblocks even if
	// the server never observed the client-side disconnect.
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)

	// Register AFTER setupServers: t.Cleanup is LIFO, so close(release) must
	// run BEFORE the httptest server's Close blocks on in-flight handlers.
	t.Cleanup(func() { close(release) })

	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Upload(ctx, client, path, Options{})
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("part upload never reached S3 within 5 s")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error")
		}
	case <-time.After(abortTimeout + time.Second):
		t.Fatal("Upload did not return after cancellation")
	}

	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1 (abort must run despite upload ctx cancellation)", mock.abortCalls.Load())
	}
}

func TestUpload_RejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	f, _ := os.Create(path)
	_ = f.Close()

	mock := &railsMock{}
	client, _ := setupServers(t, mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach S3")
	}))

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if mock.presignCalls.Load() != 0 {
		t.Errorf("presign called for empty file")
	}
}

func TestUpload_RejectsOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge")
	f, _ := os.Create(path)
	if err := f.Truncate(MaxFileSize + 1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	_ = f.Close()

	mock := &railsMock{}
	client, _ := setupServers(t, mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach S3")
	}))

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if mock.presignCalls.Load() != 0 {
		t.Errorf("presign called for oversize file")
	}
}

func TestUpload_PresignWrongPartCount(t *testing.T) {
	setTestPartSize(t, 5)
	path := writeFile(t, 12) // expects 3 parts

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach S3")
	}))
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		// Return only 2 parts instead of 3.
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1), s3URL + partPath(2)})
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1 (presign succeeded, must abort)", mock.abortCalls.Load())
	}
}

// writePresignWithPartNumbers mints a presign response with caller-supplied
// part_number values, deriving each URL from urlBase via partPath. Used to
// drive malformed-part_number scenarios through validatePresignParts.
func writePresignWithPartNumbers(t *testing.T, w http.ResponseWriter, uploadID, key, fileURL, urlBase string, numbers []int) {
	t.Helper()
	parts := make([]map[string]any, len(numbers))
	for i, n := range numbers {
		parts[i] = map[string]any{
			"part_number":   n,
			"presigned_url": urlBase + partPath(n),
		}
	}
	testutil.JSON(t, w, map[string]any{
		"upload_id": uploadID,
		"key":       key,
		"file_url":  fileURL,
		"parts":     parts,
	})
}

func TestUpload_PartTimeoutFiresOnStalledS3(t *testing.T) {
	// A peer that accepts the connection but never responds must not hang
	// the upload forever — Options.PartTimeout bounds each attempt.
	setTestPartSize(t, 100)
	shortBackoff(t)
	path := writeFile(t, 50)

	var attempts atomic.Int32
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done() // never respond; wait for client abort
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}

	_, err := Upload(context.Background(), client, path, Options{PartTimeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	// Per-part timeout is retryable, so attempts should equal maxPartRetries+1.
	if got := attempts.Load(); got != int32(maxPartRetries+1) {
		t.Errorf("attempts = %d, want %d (inner timeout should retry)", got, maxPartRetries+1)
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1", mock.abortCalls.Load())
	}
}

func TestUpload_SlowProgressCallbackDoesNotStallWorkers(t *testing.T) {
	// With Concurrency=1 and a progress callback that blocks, earlier
	// designs held the semaphore slot across the callback and froze the
	// second part. The slot is now released before the callback runs.
	setTestPartSize(t, 5)
	const fileSize int64 = 10 // 2 parts
	path := writeFile(t, fileSize)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		pn := partNumberFromPath(r.URL.Path)
		w.Header().Set("ETag", fmt.Sprintf(`"e-%d"`, pn))
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1), s3URL + partPath(2)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://f"})
	}

	var released atomic.Int32
	firstCallbackEntered := make(chan struct{})
	gateCallback := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Upload(context.Background(), client, path, Options{
			Concurrency: 1,
			Progress: func(n int64) {
				// Block the first progress invocation until the test signals.
				// Before the fix, this would prevent the second part from
				// ever acquiring a semaphore slot.
				if released.Add(1) == 1 {
					close(firstCallbackEntered)
					<-gateCallback
				}
			},
		})
		done <- err
	}()

	select {
	case <-firstCallbackEntered:
	case <-time.After(5 * time.Second):
		close(gateCallback)
		t.Fatal("progress callback never fired")
	}

	// Give the second part a chance to run independently of the blocked
	// callback. If the slot were still held, nothing would happen.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mock.completeCalls.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if mock.completeCalls.Load() == 0 {
		close(gateCallback)
		<-done
		t.Fatal("upload stalled: second part never ran while the first progress callback was blocked")
	}

	close(gateCallback)
	if err := <-done; err != nil {
		t.Fatalf("Upload: %v", err)
	}
}

func TestUpload_ErrorBodyNotDrained(t *testing.T) {
	// If S3 returns an error with a very large (or slow-streaming) body, we
	// must not block the worker draining it. The per-part timeout also
	// bounds this, but the test shows we return well before the timeout.
	setTestPartSize(t, 100)
	shortBackoff(t)
	path := writeFile(t, 50)

	// Handler writes a 5xx header and then a body that takes ~3 s to stream.
	// A drain-to-EOF would block; closing immediately on non-2xx returns fast.
	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 30; i++ {
			_, _ = w.Write([]byte("error data "))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}

	start := time.Now()
	_, err := Upload(context.Background(), client, path, Options{PartTimeout: 10 * time.Second})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error")
	}
	// Three attempts with 1 ms backoff in shortBackoff; each attempt should
	// close on the 5xx snippet and retry. Total time must be far less than
	// 3 s * 3 attempts = 9 s.
	if elapsed > 2*time.Second {
		t.Errorf("took %s — error body drain appears to be blocking", elapsed)
	}
}

func TestUpload_SuccessBodyDrainBounded(t *testing.T) {
	// A peer that returns 200 + ETag and then trickles body bytes must not
	// stall the worker. The success path closes the body instead of
	// draining to EOF.
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	s3Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"slow-etag"`)
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush() // send the header immediately
		}
		// Then trickle bytes slowly. If putPart drains to EOF on success,
		// this would block ~3s; with the fix, it returns immediately.
		for i := 0; i < 30; i++ {
			_, _ = w.Write([]byte("slow "))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	mock := &railsMock{}
	client, s3URL := setupServers(t, mock, s3Handler)
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		writePresignResponse(t, w, "u", "k", "https://f", []string{s3URL + partPath(1)})
	}
	mock.complete = func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"file_url": "https://f"})
	}

	start := time.Now()
	fileURL, err := Upload(context.Background(), client, path, Options{PartTimeout: 10 * time.Second})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if fileURL != "https://f" {
		t.Errorf("fileURL = %q", fileURL)
	}
	if elapsed > 1*time.Second {
		t.Errorf("took %s — success-path drain appears to be blocking", elapsed)
	}
}

func TestUpload_PresignPartialResponseTriggersAbort(t *testing.T) {
	// If the presign body parses upload_id/key successfully but then fails
	// — e.g. a later field has the wrong type — the server has already
	// created the multipart upload. Upload must call /files/abort using
	// the salvaged handles instead of leaking the orphan.
	setTestPartSize(t, 100)
	path := writeFile(t, 50)

	mock := &railsMock{}
	client, _ := setupServers(t, mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("S3 must not be reached when presign parsing fails")
	}))
	mock.presign = func(w http.ResponseWriter, r *http.Request) {
		// `parts` is a string instead of an array: json.Unmarshal populates
		// UploadID and Key in order, then fails on the type mismatch,
		// returning the partially-filled struct alongside the parse error.
		_, _ = io.WriteString(w, `{"success":true,"upload_id":"up-orphan","key":"attachments/orphan","parts":"not_an_array"}`)
	}

	_, err := Upload(context.Background(), client, path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse presign response") {
		t.Errorf("error = %v, want parse-presign error", err)
	}
	if mock.abortCalls.Load() != 1 {
		t.Errorf("abortCalls = %d, want 1 (orphan must be cleaned up)", mock.abortCalls.Load())
	}
	var cleanup *CleanupFailedError
	if errors.As(err, &cleanup) {
		t.Errorf("abort unexpectedly failed: %v", cleanup)
	}
}

func TestValidatePresignParts_HTTPSOnly(t *testing.T) {
	// Directly exercise the production code path (allowInsecureUploadDestination
	// set back to false) to confirm plain http URLs are rejected before any
	// PUT leaves the process.
	prev := allowInsecureUploadDestination
	allowInsecureUploadDestination = false
	t.Cleanup(func() { allowInsecureUploadDestination = prev })

	tests := []struct {
		name   string
		url    string
		wantOK bool
	}{
		{"https", "https://s3.amazonaws.com/bucket/key?x=y", true},
		{"http_plaintext", "http://s3.amazonaws.com/bucket/key", false},
		{"other_scheme", "file:///etc/passwd", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePresignParts([]presignPart{{PartNumber: 1, PresignedURL: tt.url}}, 1)
			if (err == nil) != tt.wantOK {
				t.Errorf("validatePresignParts(%q) err = %v, wantOK = %v", tt.url, err, tt.wantOK)
			}
		})
	}
}

func TestUpload_PresignPartNumberValidation(t *testing.T) {
	setTestPartSize(t, 5)
	const fileSize int64 = 12 // 3 parts

	tests := []struct {
		name    string
		numbers []int // must have length 3
	}{
		{"duplicate", []int{1, 1, 2}},
		{"gap", []int{1, 3, 4}},
		{"zero", []int{0, 1, 2}},
		{"out_of_order", []int{2, 1, 3}},
		{"negative", []int{-1, 1, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, fileSize)
			mock := &railsMock{}
			client, s3URL := setupServers(t, mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("S3 must not be reached with invalid part numbers")
			}))
			mock.presign = func(w http.ResponseWriter, r *http.Request) {
				writePresignWithPartNumbers(t, w, "u", "k", "https://f", s3URL, tt.numbers)
			}

			_, err := Upload(context.Background(), client, path, Options{})
			if err == nil {
				t.Fatal("expected error for invalid part numbers")
			}
			if mock.abortCalls.Load() != 1 {
				t.Errorf("abortCalls = %d, want 1 (presign succeeded, must abort)", mock.abortCalls.Load())
			}
		})
	}
}
