package products

import (
	"bytes"
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

	getCalls       atomic.Int32
	putCalls       atomic.Int32
	jsonPutCalls   atomic.Int32
	s3Calls        atomic.Int32
	completeSeq    atomic.Int32
	failCompleteOn int32
	putStatus      int

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

	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = s.s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

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
					s.putForm = r.PostForm
				}
				if s.putStatus != 0 {
					http.Error(w, "update failed", s.putStatus)
					return
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
			if s.failCompleteOn == seq {
				http.Error(w, "complete failed", http.StatusBadGateway)
				return
			}
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

func productUpdateJSONFiles(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	raw, ok := body["files"].([]any)
	if !ok {
		t.Fatalf("files payload has wrong type: %T", body["files"])
	}
	files := make([]map[string]any, len(raw))
	for i, current := range raw {
		file, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("files[%d] has wrong type: %T", i, current)
		}
		files[i] = file
	}
	return files
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
	if srv.jsonPutCalls.Load() != 1 {
		t.Fatalf("expected JSON PUT, got %d JSON PUTs", srv.jsonPutCalls.Load())
	}
	if srv.s3Calls.Load() != 1 {
		t.Fatalf("S3 calls = %d, want 1", srv.s3Calls.Load())
	}
	if srv.presignBody["filename"] != "New Pack.zip" {
		t.Fatalf("presign filename = %q, want New Pack.zip", srv.presignBody["filename"])
	}

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 3 {
		t.Fatalf("files payload len = %d, want 3", len(files))
	}
	if files[0]["id"] != "file_a" {
		t.Fatalf("files[0].id = %#v, want file_a", files[0]["id"])
	}
	if files[1]["id"] != "file_b" {
		t.Fatalf("files[1].id = %#v, want file_b", files[1]["id"])
	}
	if files[2]["url"] != "https://example.com/attachments/u/k/original/upload-1.bin" {
		t.Fatalf("files[2].url = %#v", files[2]["url"])
	}
	if files[2]["display_name"] != "New Pack.zip" {
		t.Fatalf("files[2].display_name = %#v", files[2]["display_name"])
	}
	if files[2]["description"] != "Updated bundle" {
		t.Fatalf("files[2].description = %#v", files[2]["description"])
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

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 1 || files[0]["id"] != "file_b" {
		t.Fatalf("files payload = %#v, want only file_b", files)
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

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 1 || files[0]["id"] != "file_b" {
		t.Fatalf("files payload = %#v, want only file_b", files)
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
	if srv.getCalls.Load() != 0 {
		t.Fatalf("unexpected GET calls: %d", srv.getCalls.Load())
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
		{ID: "file_a", Name: "Old A.pdf"},
		{ID: "file_b", Name: "Old B.pdf"},
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
		{ID: "file_a", Name: "Old A.pdf"},
		{ID: "file_b", Name: "Old B.pdf"},
	}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.JSONOutput())
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
	var payload dryRunUpdateBody
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if len(payload.Uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(payload.Uploads))
	}
	if payload.Uploads[0].Path != path {
		t.Fatalf("upload path = %q, want %q", payload.Uploads[0].Path, path)
	}
	if payload.Uploads[0].Filename != filepath.Base(path) {
		t.Fatalf("upload filename = %q, want %q", payload.Uploads[0].Filename, filepath.Base(path))
	}
	if payload.Uploads[0].Size != int64(len("fresh bytes")) {
		t.Fatalf("upload size = %d, want %d", payload.Uploads[0].Size, len("fresh bytes"))
	}
	if payload.Uploads[0].PartCount != 1 {
		t.Fatalf("upload part count = %d, want 1", payload.Uploads[0].PartCount)
	}
	if len(payload.Preserved) != 1 || payload.Preserved[0].ID != "file_b" || payload.Preserved[0].Name != "Old B.pdf" {
		t.Fatalf("preserved = %+v", payload.Preserved)
	}
	if len(payload.Removed) != 1 || payload.Removed[0].ID != "file_a" || payload.Removed[0].Name != "Old A.pdf" {
		t.Fatalf("removed = %+v", payload.Removed)
	}
	files := productUpdateJSONFiles(t, payload.Request.Body)
	if len(files) != 2 || files[0]["id"] != "file_b" {
		t.Fatalf("dry-run files payload = %#v", files)
	}
	if files[1]["url"] != "<uploaded:file:0>" {
		t.Fatalf("dry-run upload placeholder = %#v", files[1]["url"])
	}
}

func TestUpdate_FileDryRunPlainIncludesUploadPlan(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod1", "--file", path})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("plain dry-run missing upload/request lines: %q", out)
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 5 {
		t.Fatalf("plain dry-run upload row = %q", lines[0])
	}
	if fields[0] != "upload" || filepath.Base(fields[1]) != filepath.Base(path) || fields[2] != filepath.Base(path) || fields[3] != "11" || fields[4] != "1" {
		t.Fatalf("plain dry-run missing upload plan: %q", out)
	}
	if !strings.Contains(out, "PUT\t/products/prod1\t") {
		t.Fatalf("plain dry-run missing request line: %q", out)
	}
}

func TestUpdate_FileDryRunHumanIncludesUploadPlan(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"prod1", "--file", path})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Dry run: upload "+path) {
		t.Fatalf("human dry-run missing upload line: %q", out)
	}
	if !strings.Contains(out, "Filename: "+filepath.Base(path)) {
		t.Fatalf("human dry-run missing filename: %q", out)
	}
	if !strings.Contains(out, "Size: 11 B (1 part)") {
		t.Fatalf("human dry-run missing size/part count: %q", out)
	}
	if !strings.Contains(out, "Dry run: PUT /products/prod1") {
		t.Fatalf("human dry-run missing request line: %q", out)
	}
}

func TestUpdate_DryRunHumanIncludesExistingFileNames(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a", Name: "Old A.pdf"},
		{ID: "file_b", Name: "Old B.pdf"},
	}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"prod1", "--remove-file", "file_a"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Preserve existing file: Old B.pdf (file_b)") {
		t.Fatalf("human dry-run missing preserved file: %q", out)
	}
	if !strings.Contains(out, "Remove existing file: Old A.pdf (file_a)") {
		t.Fatalf("human dry-run missing removed file: %q", out)
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
	if srv.getCalls.Load() != 0 {
		t.Fatalf("unexpected GET calls: %d", srv.getCalls.Load())
	}
}

func TestCollectRequestedProductUploads_RequiresFileForMetadataFlags(t *testing.T) {
	cmd := newUpdateCmd()

	_, err := collectRequestedProductUploads(cmd, nil, []string{" Gift.zip "}, nil)
	if err == nil || !strings.Contains(err.Error(), "--file-name requires at least one --file") {
		t.Fatalf("expected --file-name usage error, got %v", err)
	}

	_, err = collectRequestedProductUploads(cmd, nil, nil, []string{"Bonus"})
	if err == nil || !strings.Contains(err.Error(), "--file-description requires at least one --file") {
		t.Fatalf("expected --file-description usage error, got %v", err)
	}
}

func TestCollectRequestedProductUploads_TrimsDisplayName(t *testing.T) {
	cmd := newUpdateCmd()

	uploads, err := collectRequestedProductUploads(cmd, []string{"./pack.zip"}, []string{"  Gift.zip  "}, []string{""})
	if err != nil {
		t.Fatalf("collectRequestedProductUploads: %v", err)
	}
	if len(uploads) != 1 || uploads[0].DisplayName != "Gift.zip" {
		t.Fatalf("uploads = %+v", uploads)
	}
}

func TestUpdate_InvalidUploadFailsBeforeRemovalConfirmation(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	missingPath := filepath.Join(t.TempDir(), "missing.bin")
	cmd := testutil.Command(newUpdateCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"prod1", "--replace-files", "--file", missingPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected local file validation error")
	}
	if !strings.Contains(err.Error(), "could not stat file") {
		t.Fatalf("expected stat error, got %v", err)
	}
	if strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected validation to happen before confirmation, got %v", err)
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
	if srv.s3Calls.Load() != 0 {
		t.Fatalf("unexpected S3 calls: %d", srv.s3Calls.Load())
	}
}

func TestUpdate_FileUploadFailureIncludesUploadedURLs(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.failCompleteOn = 2
	testutil.Setup(t, srv.dispatch(t))

	firstPath := writeProductUploadFixture(t, "first")
	secondPath := writeProductUploadFixture(t, "second")
	cmd := testutil.Command(newUpdateCmd())
	cmd.SetArgs([]string{
		"prod1",
		"--file", firstPath,
		"--file", secondPath,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected upload failure")
	}
	if !strings.Contains(err.Error(), "https://example.com/attachments/u/k/original/upload-1.bin") {
		t.Fatalf("expected uploaded URL in error, got %v", err)
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
}

func TestUpdate_ProductUpdateFailureIncludesUploadedURLs(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.putStatus = http.StatusBadGateway
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "first")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected update failure")
	}
	if !strings.Contains(err.Error(), "https://example.com/attachments/u/k/original/upload-1.bin") {
		t.Fatalf("expected uploaded URL in error, got %v", err)
	}
	if srv.putCalls.Load() != 1 {
		t.Fatalf("PUT calls = %d, want 1", srv.putCalls.Load())
	}
}

func TestUpdate_ReplaceFilesClearAllDryRunJSON(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--replace-files"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var payload dryRunUpdateBody
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if payload.Request.Method != http.MethodPut || payload.Request.Path != "/products/prod1" {
		t.Fatalf("unexpected dry-run envelope: %+v", payload)
	}
	if len(payload.Uploads) != 0 {
		t.Fatalf("expected no uploads, got %+v", payload.Uploads)
	}
	if len(payload.Preserved) != 0 {
		t.Fatalf("expected no preserved files, got %+v", payload.Preserved)
	}
	if len(payload.Removed) != 1 || payload.Removed[0].ID != "file_a" {
		t.Fatalf("removed = %+v", payload.Removed)
	}
	files, ok := payload.Request.Body["files"].([]any)
	if !ok {
		t.Fatalf("files payload has wrong type: %T", payload.Request.Body["files"])
	}
	if len(files) != 0 {
		t.Fatalf("expected empty files array, got %#v", files)
	}
}

func TestUpdate_ReplaceFilesClearAllDryRunPlain(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod1", "--replace-files"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "PUT\t/products/prod1\t") {
		t.Fatalf("plain dry-run missing request line: %q", out)
	}
	if !strings.Contains(out, "\"files\":[]") {
		t.Fatalf("plain dry-run missing empty files array: %q", out)
	}
}

func TestUpdate_ReplaceFilesClearAllDryRunHuman(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"prod1", "--replace-files"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Dry run: PUT /products/prod1") {
		t.Fatalf("human dry-run missing request line: %q", out)
	}
	if !strings.Contains(out, "\"files\": []") {
		t.Fatalf("human dry-run missing empty files array: %q", out)
	}
}

func TestBuildProductJSONBody_MapsTagsAndRepeatedValues(t *testing.T) {
	params := url.Values{
		"name":   {"Updated"},
		"tags[]": {"art", "digital"},
		"other":  {"one", "two"},
	}
	files := []map[string]any{{"id": "file_a"}}

	body := buildProductJSONBody(params, files)
	if got := body["name"]; got != "Updated" {
		t.Fatalf("name = %#v, want Updated", got)
	}
	tags, ok := body["tags"].([]string)
	if !ok || len(tags) != 2 || tags[0] != "art" || tags[1] != "digital" {
		t.Fatalf("tags = %#v", body["tags"])
	}
	other, ok := body["other"].([]string)
	if !ok || len(other) != 2 || other[0] != "one" || other[1] != "two" {
		t.Fatalf("other = %#v", body["other"])
	}
	gotFiles, ok := body["files"].([]map[string]any)
	if !ok || len(gotFiles) != 1 || gotFiles[0]["id"] != "file_a" {
		t.Fatalf("files = %#v", body["files"])
	}
}

func TestWrapPartialUploadErrorIncludesUploadedURLs(t *testing.T) {
	err := wrapPartialUploadError(fmt.Errorf("boom"), []string{"one", "two"})
	if !strings.Contains(err.Error(), "one") || !strings.Contains(err.Error(), "two") {
		t.Fatalf("wrapped error = %v", err)
	}
}

func TestWrapPartialUploadErrorNilStaysNil(t *testing.T) {
	if err := wrapPartialUploadError(nil, []string{"one"}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestProductFileRemovalMessageIncludesNames(t *testing.T) {
	message := productFileRemovalMessage("prod1", []existingProductFile{
		{ID: "file_a", Name: "Old A.pdf"},
		{ID: "file_b", Name: "Old B.pdf"},
		{ID: "file_c"},
	})
	if !strings.Contains(message, "Update product prod1 and remove 3 existing files:") {
		t.Fatalf("unexpected message: %q", message)
	}
	if !strings.Contains(message, "Old A.pdf (file_a)") || !strings.Contains(message, "Old B.pdf (file_b)") {
		t.Fatalf("unexpected message: %q", message)
	}
	if !strings.Contains(message, "file_c") {
		t.Fatalf("unexpected summary: %q", message)
	}
}

func TestRenderProductUpdateDryRunJSON_Direct(t *testing.T) {
	var buf bytes.Buffer
	opts := testutil.TestOptions(testutil.Stdout(&buf), testutil.JSONOutput())
	body := map[string]any{"files": []map[string]any{}}

	if err := renderProductUpdateDryRunJSON(opts, "/products/prod1", productFileUpdatePlan{}, nil, body); err != nil {
		t.Fatalf("renderProductUpdateDryRunJSON: %v", err)
	}
	var payload dryRunUpdateBody
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("parse output: %v\n%s", err, buf.String())
	}
	if payload.Request.Path != "/products/prod1" || payload.Request.Method != http.MethodPut || !payload.DryRun {
		t.Fatalf("unexpected JSON dry-run output: %+v", payload)
	}
	if len(payload.Uploads) != 0 {
		t.Fatalf("expected no uploads, got %+v", payload.Uploads)
	}
	if len(payload.Preserved) != 0 || len(payload.Removed) != 0 {
		t.Fatalf("unexpected file delta: preserved=%+v removed=%+v", payload.Preserved, payload.Removed)
	}
}
