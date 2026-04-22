package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

type productUpdateFileServers struct {
	existingFiles []existingProductFile

	s3 *httptest.Server

	getCalls     atomic.Int32
	putCalls     atomic.Int32
	jsonPutCalls atomic.Int32
	s3Calls      atomic.Int32
	completeSeq  atomic.Int32

	putForm     url.Values
	putJSON     map[string]any
	presignBody map[string]string
}

func newProductUpdateFileServers(t *testing.T) *productUpdateFileServers {
	t.Helper()

	s := &productUpdateFileServers{
		presignBody: map[string]string{},
	}
	s.s3 = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.s3Calls.Add(1)
		if r.Method != http.MethodPut {
			t.Errorf("S3 got %s, want PUT", r.Method)
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.s3.Close)

	prev := productUploadHTTPClientForTesting
	productUploadHTTPClientForTesting = s.s3.Client()
	t.Cleanup(func() { productUploadHTTPClientForTesting = prev })

	return s
}

func (s *productUpdateFileServers) dispatch(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/prod1":
			switch r.Method {
			case http.MethodGet:
				s.getCalls.Add(1)
				testutil.JSON(t, w, map[string]any{
					"product": map[string]any{
						"id":    "prod1",
						"files": s.existingFiles,
					},
				})
			case http.MethodPut:
				s.putCalls.Add(1)
				if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					s.jsonPutCalls.Add(1)
					if err := json.NewDecoder(r.Body).Decode(&s.putJSON); err != nil {
						t.Fatalf("decode JSON body: %v", err)
					}
				} else {
					if err := r.ParseForm(); err != nil {
						t.Fatalf("ParseForm failed: %v", err)
					}
					s.putForm = cmdValuesClone(r.PostForm)
				}
				testutil.JSON(t, w, map[string]any{})
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		case "/files/presign":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm failed: %v", err)
			}
			for key := range r.PostForm {
				s.presignBody[key] = r.PostForm.Get(key)
			}
			testutil.JSON(t, w, map[string]any{
				"upload_id": "up-1",
				"key":       "attachments/u/k/original/upload.bin",
				"file_url":  "https://example.com/attachments/u/k/original/upload.bin",
				"parts": []map[string]any{
					{"part_number": 1, "presigned_url": s.s3.URL + "/part/1"},
				},
			})
		case "/files/complete":
			seq := s.completeSeq.Add(1)
			testutil.JSON(t, w, map[string]any{
				"file_url": fmt.Sprintf("https://example.com/attachments/u/k/original/upload-%d.bin", seq),
			})
		case "/files/abort":
			testutil.JSON(t, w, map[string]any{})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

func writeProductUploadFixture(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func cmdValuesClone(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, current := range values {
		cloned[key] = append([]string(nil), current...)
	}
	return cloned
}

func TestUpdate_FilePreservesExistingByDefault(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a", Name: "Old A"},
		{ID: "file_b", Name: "Old B"},
	}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
		"--file-description", "Updated bundle",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.getCalls.Load() != 1 {
		t.Fatalf("GET calls = %d, want 1", srv.getCalls.Load())
	}
	if srv.putCalls.Load() != 1 {
		t.Fatalf("PUT calls = %d, want 1", srv.putCalls.Load())
	}
	if srv.jsonPutCalls.Load() != 0 {
		t.Fatalf("expected form PUT, got %d JSON PUTs", srv.jsonPutCalls.Load())
	}
	if srv.s3Calls.Load() != 1 {
		t.Fatalf("S3 calls = %d, want 1", srv.s3Calls.Load())
	}
	if srv.presignBody["filename"] != "New Pack.zip" {
		t.Fatalf("presign filename = %q, want New Pack.zip", srv.presignBody["filename"])
	}

	if got := srv.putForm.Get("files[0][id]"); got != "file_a" {
		t.Fatalf("files[0][id] = %q, want file_a", got)
	}
	if got := srv.putForm.Get("files[1][id]"); got != "file_b" {
		t.Fatalf("files[1][id] = %q, want file_b", got)
	}
	if got := srv.putForm.Get("files[2][url]"); got != "https://example.com/attachments/u/k/original/upload-1.bin" {
		t.Fatalf("files[2][url] = %q", got)
	}
	if got := srv.putForm.Get("files[2][display_name]"); got != "New Pack.zip" {
		t.Fatalf("files[2][display_name] = %q", got)
	}
	if got := srv.putForm.Get("files[2][description]"); got != "Updated bundle" {
		t.Fatalf("files[2][description] = %q", got)
	}
}

func TestUpdate_RemoveFilePreservesOthers(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a"},
		{ID: "file_b"},
	}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1", "--remove-file", "file_a"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := srv.putForm.Get("files[0][id]"); got != "file_b" {
		t.Fatalf("files[0][id] = %q, want file_b", got)
	}
	if got := srv.putForm.Get("files[1][id]"); got != "" {
		t.Fatalf("unexpected second preserved file: %q", got)
	}
}

func TestUpdate_ReplaceFilesKeepsOnlyRequestedIDs(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a"},
		{ID: "file_b"},
		{ID: "file_c"},
	}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1", "--replace-files", "--keep-file", "file_b"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := srv.putForm.Get("files[0][id]"); got != "file_b" {
		t.Fatalf("files[0][id] = %q, want file_b", got)
	}
	if got := srv.putForm.Get("files[1][id]"); got != "" {
		t.Fatalf("unexpected extra preserved file: %q", got)
	}
}

func TestUpdate_KeepAndRemoveSameIDErrors(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"prod1", "--replace-files", "--keep-file", "file_a", "--remove-file", "file_a"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "--keep-file") || !strings.Contains(err.Error(), "--remove-file") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
}

func TestUpdate_UnknownRemoveFileErrorsAfterPrefetch(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"prod1", "--remove-file", "missing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "unknown --remove-file") {
		t.Fatalf("expected unknown remove-file error, got %v", err)
	}
	if srv.getCalls.Load() != 1 {
		t.Fatalf("GET calls = %d, want 1", srv.getCalls.Load())
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
}

func TestUpdate_ReplaceFilesClearAllUsesJSONBody(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a"},
		{ID: "file_b"},
	}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1", "--replace-files"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.jsonPutCalls.Load() != 1 {
		t.Fatalf("JSON PUT calls = %d, want 1", srv.jsonPutCalls.Load())
	}
	files, ok := srv.putJSON["files"].([]any)
	if !ok {
		t.Fatalf("files payload has wrong type: %T", srv.putJSON["files"])
	}
	if len(files) != 0 {
		t.Fatalf("files payload = %#v, want empty array", files)
	}
}

func TestUpdate_ReplaceFilesNoInputRequiresYes(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"prod1", "--replace-files"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes hint, got %v", err)
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
}

func TestUpdate_FileDryRunPrefetchesButDoesNotUploadOrPut(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a"},
		{ID: "file_b"},
	}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"prod1", "--file", path, "--remove-file", "file_a"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.getCalls.Load() != 1 {
		t.Fatalf("GET calls = %d, want 1", srv.getCalls.Load())
	}
	if srv.s3Calls.Load() != 0 {
		t.Fatalf("unexpected S3 calls: %d", srv.s3Calls.Load())
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
	if !strings.Contains(out, "files[0][id]: file_b") {
		t.Fatalf("dry-run output missing preserved id: %q", out)
	}
	if !strings.Contains(out, "<uploaded:file:0>") {
		t.Fatalf("dry-run output missing placeholder upload URL: %q", out)
	}
}

func TestUpdate_KeepFileRequiresReplaceFiles(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"prod1", "--keep-file", "file_a"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "--replace-files") {
		t.Fatalf("expected keep-file usage error, got %v", err)
	}
}
