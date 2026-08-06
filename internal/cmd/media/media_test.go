package media

import (
	"bytes"
	"crypto/md5" // #nosec G501 -- verifying the S3 Content-MD5 integrity header in tests.
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

// pngBytes is a minimal valid PNG header + IHDR chunk — enough for
// http.DetectContentType to report image/png.
var pngBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89,
}

type mediaServers struct {
	s3      *httptest.Server
	s3Calls atomic.Int32

	mu               sync.Mutex
	directUploadForm map[string]string
	mediaForm        map[string]string
	s3Headers        map[string]string
	s3BodyLen        int64
	railsReached     atomic.Bool
}

func newMediaServers(t *testing.T) *mediaServers {
	t.Helper()
	m := &mediaServers{}
	m.s3 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.s3Calls.Add(1)
		if r.Method != http.MethodPut {
			t.Errorf("S3 got %s, want PUT", r.Method)
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		n, _ := io.Copy(io.Discard, r.Body)
		m.mu.Lock()
		m.s3Headers = map[string]string{
			"Content-Type": r.Header.Get("Content-Type"),
			"Content-MD5":  r.Header.Get("Content-MD5"),
		}
		m.s3BodyLen = n
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(m.s3.Close)

	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = m.s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

	return m
}

func (m *mediaServers) dispatch(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		m.railsReached.Store(true)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/direct_uploads":
			if r.Method != http.MethodPost {
				t.Errorf("/direct_uploads got %s, want POST", r.Method)
			}
			m.mu.Lock()
			m.directUploadForm = map[string]string{
				"purpose":      r.PostForm.Get("purpose"),
				"filename":     r.PostForm.Get("blob[filename]"),
				"byte_size":    r.PostForm.Get("blob[byte_size]"),
				"checksum":     r.PostForm.Get("blob[checksum]"),
				"content_type": r.PostForm.Get("blob[content_type]"),
			}
			m.mu.Unlock()
			testutil.JSON(t, w, map[string]any{
				"signed_id": "signed-1",
				"direct_upload": map[string]any{
					"url":     m.s3.URL + "/upload/1",
					"headers": map[string]string{"Content-Type": "image/png"},
				},
			})
		case "/media":
			if r.Method != http.MethodPost {
				t.Errorf("/media got %s, want POST", r.Method)
			}
			m.mu.Lock()
			m.mediaForm = map[string]string{
				"signed_blob_id": r.PostForm.Get("signed_blob_id"),
				"name":           r.PostForm.Get("name"),
			}
			m.mu.Unlock()
			testutil.JSON(t, w, map[string]any{
				"media": map[string]any{
					"id":        "G_abc123",
					"name":      "logo.png",
					"url":       "https://public-files.gumroad.com/abc123.png",
					"file_size": len(pngBytes),
				},
			})
		default:
			t.Errorf("unexpected Rails path: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

func writePNGFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(path, pngBytes, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestMediaUpload_HappyPath(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writePNGFixture(t)
	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.Quiet(false), testutil.Stdout(&out))
	cmd.SetArgs([]string{path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := srv.s3Calls.Load(); got != 1 {
		t.Fatalf("s3 calls = %d, want 1", got)
	}
	srv.mu.Lock()
	form := srv.directUploadForm
	mediaForm := srv.mediaForm
	srv.mu.Unlock()
	if form["purpose"] != "media" {
		t.Fatalf("direct upload purpose = %q, want media", form["purpose"])
	}
	if form["content_type"] != "image/png" {
		t.Fatalf("content_type = %q, want image/png", form["content_type"])
	}
	if form["filename"] != "logo.png" {
		t.Fatalf("filename = %q, want logo.png", form["filename"])
	}
	if mediaForm["signed_blob_id"] != "signed-1" {
		t.Fatalf("signed_blob_id = %q, want signed-1", mediaForm["signed_blob_id"])
	}
	wantMD5 := md5.Sum(pngBytes) // #nosec G401 -- verifying the S3 Content-MD5 integrity header.
	wantChecksum := base64.StdEncoding.EncodeToString(wantMD5[:])
	if form["checksum"] != wantChecksum {
		t.Fatalf("presign checksum = %q, want %q", form["checksum"], wantChecksum)
	}
	srv.mu.Lock()
	s3h, s3n := srv.s3Headers, srv.s3BodyLen
	srv.mu.Unlock()
	if s3h["Content-MD5"] != wantChecksum {
		t.Fatalf("S3 Content-MD5 = %q, want %q", s3h["Content-MD5"], wantChecksum)
	}
	if s3h["Content-Type"] != "image/png" {
		t.Fatalf("S3 Content-Type = %q, want image/png", s3h["Content-Type"])
	}
	if s3n != int64(len(pngBytes)) {
		t.Fatalf("S3 body length = %d, want %d", s3n, len(pngBytes))
	}
	if !strings.Contains(out.String(), "https://public-files.gumroad.com/abc123.png") {
		t.Fatalf("output missing URL: %q", out.String())
	}
}

func TestMediaUpload_NamePassedThrough(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{writePNGFixture(t), "--name", "Store logo"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.mediaForm["name"] != "Store logo" {
		t.Fatalf("name = %q, want Store logo", srv.mediaForm["name"])
	}
}

func TestMediaUpload_JSONOutput(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.JSONOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var decoded mediaUploadResponse
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON output not parseable: %v (%q)", err, out.String())
	}
	if decoded.Media.URL != "https://public-files.gumroad.com/abc123.png" {
		t.Fatalf("media.url = %q", decoded.Media.URL)
	}
}

func TestMediaUpload_RejectsNonImageBeforeRequest(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("plain text"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not an image") {
		t.Fatalf("err = %v, want not-an-image", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a rejected file")
	}
}

func TestMediaUpload_RejectsSVGBeforeRequest(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(path, []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "SVG") {
		t.Fatalf("err = %v, want SVG rejection", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a rejected file")
	}
}

func TestMediaUpload_RejectsOversizeBeforeRequest(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := filepath.Join(t.TempDir(), "big.png")
	big := make([]byte, maxMediaImageBytes+1)
	copy(big, pngBytes)
	if err := os.WriteFile(path, big, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "10 MB") {
		t.Fatalf("err = %v, want size cap", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a rejected file")
	}
}

func TestMediaUpload_RejectsEmptyFileBeforeRequest(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want empty rejection", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a rejected file")
	}
}

func TestMediaUpload_DryRunMakesNoRequests(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.DryRun(true), testutil.JSONOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.railsReached.Load() || srv.s3Calls.Load() != 0 {
		t.Fatal("dry run made real requests")
	}
	var payload dryRunUploadPlan
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("dry-run JSON not parseable: %v (%q)", err, out.String())
	}
	if !payload.DryRun || payload.ContentType != "image/png" {
		t.Fatalf("dry-run payload = %#v", payload)
	}
}

func TestMediaList_RendersRows(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media" || r.Method != http.MethodGet {
			t.Errorf("got %s %s, want GET /media", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"media": []map[string]any{
				{"id": "G_1", "name": "logo.png", "url": "https://public-files.gumroad.com/1.png", "file_size": 1234},
			},
		})
	})

	var out bytes.Buffer
	cmd := testutil.Command(newListCmd(), testutil.PlainOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "G_1") || !strings.Contains(out.String(), "https://public-files.gumroad.com/1.png") {
		t.Fatalf("plain output = %q", out.String())
	}
}

func TestMediaList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"media": []map[string]any{}})
	})

	var out bytes.Buffer
	cmd := testutil.Command(newListCmd(), testutil.Quiet(false), testutil.Stdout(&out))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "No media library files found.") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestMediaDelete_SendsDelete(t *testing.T) {
	var gotPath, gotMethod string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		testutil.JSON(t, w, map[string]any{"message": "The file was deleted."})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"k3n8xq1p9wr2sd4a"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/media/k3n8xq1p9wr2sd4a" {
		t.Fatalf("got %s %s, want DELETE /media/k3n8xq1p9wr2sd4a", gotMethod, gotPath)
	}
}

func TestMediaUpload_MissingSignedID(t *testing.T) {
	newMediaServers(t)
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"direct_upload": map[string]any{"url": "https://example.com/u"}})
	})

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{writePNGFixture(t)})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "signed_id") {
		t.Fatalf("err = %v, want signed_id error", err)
	}
}

func TestMediaUpload_MissingUploadURL(t *testing.T) {
	newMediaServers(t)
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"signed_id": "signed-1"})
	})

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{writePNGFixture(t)})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "upload URL") {
		t.Fatalf("err = %v, want upload URL error", err)
	}
}

func TestMediaUpload_S3FailureSurfacesBody(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "access denied", http.StatusForbidden)
	}))
	t.Cleanup(s3.Close)
	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

	mediaReached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media" {
			mediaReached = true
		}
		testutil.JSON(t, w, map[string]any{
			"signed_id":     "signed-1",
			"direct_upload": map[string]any{"url": s3.URL + "/upload/1"},
		})
	})

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{writePNGFixture(t)})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("err = %v, want HTTP 403 with body", err)
	}
	if mediaReached {
		t.Fatal("POST /media fired after a failed direct upload")
	}
}

func TestMediaUpload_ServerRejectionSurfaced(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media" {
			testutil.RawJSON(t, w, `{"success": false, "message": "This image was flagged by content moderation."}`)
			return
		}
		srv.dispatch(t)(w, r)
	})

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{writePNGFixture(t)})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "flagged by content moderation") {
		t.Fatalf("err = %v, want moderation message", err)
	}
}

func TestMediaUpload_PlainOutput(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.PlainOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "G_abc123") || !strings.Contains(out.String(), "https://public-files.gumroad.com/abc123.png") {
		t.Fatalf("plain output = %q", out.String())
	}
}

func TestMediaUpload_DryRunPlainOutput(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.DryRun(true), testutil.PlainOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("dry run made real requests")
	}
	if !strings.Contains(out.String(), "media upload") || !strings.Contains(out.String(), "image/png") {
		t.Fatalf("plain dry-run output = %q", out.String())
	}
}

func TestMediaUpload_DryRunHumanOutput(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.DryRun(true), testutil.Quiet(false), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("dry run made real requests")
	}
	if !strings.Contains(out.String(), "Dry run") || !strings.Contains(out.String(), "Content type: image/png") {
		t.Fatalf("human dry-run output = %q", out.String())
	}
}

func TestMediaUpload_MissingFileErrors(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "nope.png")})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not open") {
		t.Fatalf("err = %v, want could-not-open", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a missing file")
	}
}

func TestMediaUpload_DirectoryErrors(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{t.TempDir()})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("err = %v, want directory rejection", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a directory")
	}
}

func TestMediaDelete_Cancelled(t *testing.T) {
	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"k3n8xq1p9wr2sd4a"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "confirmation required") {
		t.Fatalf("err = %v, want confirmation-required", err)
	}
	if reached {
		t.Fatal("unconfirmed delete reached the API")
	}
}

func TestMediaList_JSONOutput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"media": []map[string]any{{"id": "G_1", "url": "https://public-files.gumroad.com/1.png"}},
		})
	})

	var out bytes.Buffer
	cmd := testutil.Command(newListCmd(), testutil.JSONOutput(), testutil.Stdout(&out))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var decoded mediaListResponse
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON output not parseable: %v (%q)", err, out.String())
	}
	if len(decoded.Media) != 1 || decoded.Media[0].ID != "G_1" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestNewMediaCmd_RegistersSubcommands(t *testing.T) {
	cmd := NewMediaCmd()
	var names []string
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	for _, want := range []string{"upload", "list", "delete"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("subcommand %q not registered (got %v)", want, names)
		}
	}
}

func TestMediaList_HumanTable(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"media": []map[string]any{
				{"id": "G_1", "name": "logo.png", "url": "https://public-files.gumroad.com/1.png", "file_size": 1234},
			},
		})
	})

	var out bytes.Buffer
	cmd := testutil.Command(newListCmd(), testutil.Quiet(false), testutil.Stdout(&out))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"G_1", "logo.png", "https://public-files.gumroad.com/1.png"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("table output missing %q: %q", want, out.String())
		}
	}
}

func TestMediaUpload_QuietSucceedsSilently(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.Quiet(true), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("quiet output = %q, want empty", out.String())
	}
}

func TestMediaUpload_DryRunQuiet(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	var out bytes.Buffer
	cmd := testutil.Command(newUploadCmd(), testutil.DryRun(true), testutil.Quiet(true), testutil.Stdout(&out))
	cmd.SetArgs([]string{writePNGFixture(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.railsReached.Load() || out.Len() != 0 {
		t.Fatalf("quiet dry run: reached=%v output=%q", srv.railsReached.Load(), out.String())
	}
}

func TestMediaUpload_JPEGAccepted(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	// Minimal JPEG magic bytes for http.DetectContentType.
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 16)...)
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, jpeg, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.directUploadForm["content_type"] != "image/jpeg" {
		t.Fatalf("content_type = %q, want image/jpeg", srv.directUploadForm["content_type"])
	}
}

func TestMediaUpload_BMPAccepted(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	// BMP magic bytes; the server's media pipeline accepts any image type
	// except SVG, so the CLI must not be stricter than it.
	bmp := append([]byte("BM"), bytes.Repeat([]byte{0}, 32)...)
	path := filepath.Join(t.TempDir(), "chart.bmp")
	if err := os.WriteFile(path, bmp, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.directUploadForm["content_type"] != "image/bmp" {
		t.Fatalf("content_type = %q, want image/bmp", srv.directUploadForm["content_type"])
	}
}

func TestMediaUpload_WebPAccepted(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	webp := append([]byte("RIFF"), append([]byte{0x20, 0, 0, 0}, append([]byte("WEBPVP8 "), bytes.Repeat([]byte{0}, 16)...)...)...)
	path := filepath.Join(t.TempDir(), "photo.webp")
	if err := os.WriteFile(path, webp, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.directUploadForm["content_type"] != "image/webp" {
		t.Fatalf("content_type = %q, want image/webp", srv.directUploadForm["content_type"])
	}
}

func TestMediaUpload_RenamedSVGStillRejected(t *testing.T) {
	srv := newMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	// An SVG renamed to .png dodges the extension guard but sniffs as XML/text,
	// so the image/ prefix check rejects it.
	path := filepath.Join(t.TempDir(), "sneaky.png")
	if err := os.WriteFile(path, []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd := testutil.Command(newUploadCmd())
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not an image") {
		t.Fatalf("err = %v, want not-an-image", err)
	}
	if srv.railsReached.Load() {
		t.Fatal("API was reached for a rejected file")
	}
}
