//go:build (darwin && !ios) || (linux && !android)

package products

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func downloadURLPayload(signedURL string, mutators ...func(map[string]any)) map[string]any {
	payload := map[string]any{
		"signed_url":    signedURL,
		"external_link": false,
		"file":          map[string]any{"id": "f_1", "display_name": "Big Guide", "file_name": "big-guide.pdf", "extension": "PDF", "filegroup": "document", "file_size": 11, "created_at": "2026-05-01T12:00:00Z", "deleted_at": nil},
	}
	for _, mutate := range mutators {
		mutate(payload)
	}
	return payload
}
func newDownloadTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	srv := httptest.NewTLSServer(handler)
	previousClient := downloadHTTPClient
	client := srv.Client()
	client.CheckRedirect = refuseDownloadRedirects
	downloadHTTPClient = client
	t.Cleanup(func() { downloadHTTPClient = previousClient })
	t.Cleanup(srv.Close)
	return srv
}
func storageServer(t *testing.T, body string) (*httptest.Server, *bool) {
	reached := new(bool)
	srv := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		*reached = r.Header.Get("Accept-Encoding") == "identity"
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write([]byte(body))
	})
	return srv, reached
}

type slowFirstWriter struct {
	delay time.Duration
}
type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func (w *slowFirstWriter) Write(p []byte) (int, error) {
	if w.delay > 0 {
		time.Sleep(w.delay)
		w.delay = 0
	}
	return len(p), nil
}

func must(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, ok bool, format string, args ...any) {
	if !ok {
		t.Fatalf(format, args...)
	}
}
func implicitDownloadPath() string { return filepath.Join(reviewDirectoryName, "file-f_1.download") }
func executeDownload(args []string, mutators ...testutil.OptionsMutator) error {
	cmd := testutil.Command(newFilesDownloadCmd(), mutators...)
	cmd.SetArgs(args)
	return cmd.Execute()
}
func TestFilesDownloadWritesTrustedPrivateDestination(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, _ := storageServer(t, "hello bytes")
	var gotPath string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed", func(p map[string]any) {
			p["file"].(map[string]any)["file_name"] = "sitecustomize.py"
			p["file"].(map[string]any)["display_name"] = "Evil\nPack\x1b[31m"
		}))
	})
	testutil.CaptureStdout(func() { must(t, executeDownload([]string{"abc123", "f_1"}, testutil.Quiet(false))) })
	check(t, gotPath == "/internal/admin/products/abc123/files/f_1/download_url", "unexpected admin path: %s", gotPath)
	data, err := os.ReadFile(filepath.Join(reviewDirectoryName, "file-f_1.download"))
	check(t, err == nil && string(data) == "hello bytes", "unexpected downloaded file: %q, %v", data, err)
	downloaded, err := os.Stat(implicitDownloadPath())
	check(t, err == nil && downloaded.Mode().Perm() == 0o600, "download mode was not private: %v", err)
	reviewDir, err := os.Stat(reviewDirectoryName)
	check(t, err == nil && reviewDir.Mode().Perm() == 0o700, "review directory mode was not private: %v", err)
	_, err = os.Stat("sitecustomize.py")
	check(t, err != nil, "seller filename was used as executable local path")
}
func TestFilesDownloadRefusesExistingImplicitDestinationBeforeStorageRequest(t *testing.T) {
	t.Chdir(t.TempDir())
	must(t, os.Mkdir(reviewDirectoryName, 0o700))
	must(t, os.WriteFile(implicitDownloadPath(), []byte("old"), 0o600))
	storage, reached := storageServer(t, "new")
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	err := executeDownload([]string{"abc123", "f_1"})
	check(t, err != nil && strings.Contains(err.Error(), "already exists"), "expected already-exists error, got: %v", err)
	check(t, !*reached, "storage was fetched despite the resolved destination guard")
	data, _ := os.ReadFile(implicitDownloadPath())
	check(t, string(data) == "old", "existing file was modified: %q", data)
}
func TestFilesDownloadRefusesDestinationAppearingMidDownload(t *testing.T) {
	t.Chdir(t.TempDir())
	storage := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(implicitDownloadPath(), []byte("raced"), 0o600); err != nil {
			t.Errorf("seed raced destination: %v", err)
			return
		}
		_, _ = w.Write([]byte("new bytes"))
	})
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	err := executeDownload([]string{"abc123", "f_1"})
	check(t, err != nil && strings.Contains(err.Error(), "already exists"), "expected already-exists error, got: %v", err)
	data, _ := os.ReadFile(implicitDownloadPath())
	check(t, string(data) == "raced", "raced file was overwritten: %q", data)
}
func TestDownloadFileHelpers(t *testing.T) {
	dir := t.TempDir()
	tmpName, dest := filepath.Join(dir, "tmp"), filepath.Join(dir, "dest")
	must(t, os.WriteFile(tmpName, []byte("second"), 0o600))
	staged, err := os.OpenFile(tmpName, os.O_RDWR, 0)
	must(t, err)
	must(t, installDownloadedFile(staged, dest, true))
	tmpName, dest = filepath.Join(dir, "tmp2"), filepath.Join(dir, "dest2")
	must(t, os.WriteFile(tmpName, []byte("next"), 0o600))
	staged, err = os.OpenFile(tmpName, os.O_RDWR, 0)
	must(t, err)
	must(t, installDownloadedFile(staged, dest, false))
	got := trustedReviewFilename("../")
	check(t, got == "file-unknown.download", "unexpected trusted name: %q", got)
	got = trustedReviewFilename(strings.Repeat("a", maxReviewFileIDLength+1))
	check(t, len(got) == maxReviewFileIDLength+len("file-.download"), "trusted name was not bounded: %q", got)
	must(t, renderDownloadSuccess(cmdutil.Options{Quiet: true}, productFile{}, "dest"))
	must(t, renderDownloadSuccess(cmdutil.Options{PlainOutput: true, Stdout: io.Discard}, productFile{}, "dest"))
	must(t, renderDownloadSuccess(cmdutil.Options{Stdout: io.Discard}, productFile{ID: "id"}, "dest"))
	check(t, renderDownloadSuccess(cmdutil.Options{Stdout: closedWriter{}}, productFile{}, "dest") != nil, "expected output error")
	closed, err := os.Open(dest)
	check(t, err == nil && verifyPrivateMode(closed, 0o700) != nil, "expected a private-mode mismatch: %v", err)
	must(t, closed.Close())
	check(t, installDownloadedFile(closed, filepath.Join(dir, "unused"), false) != nil, "expected closed-file error")
	check(t, verifyPrivateMode(closed, 0o600) != nil && makeOpenPathPrivate(closed, 0o600) != nil, "expected closed-file errors")
	check(t, preparePrivateDirectory(dest) != nil, "expected a non-directory review path error")
	_, _, err = lockPrivateDirectory(filepath.Join(dir, "missing"), false)
	check(t, err != nil, "expected missing-directory error")
	_, _, _, err = prepareDownloadStaging(filepath.Join(dest, "child"))
	check(t, err != nil, "expected non-directory parent error")
	_, err = stageDownloadOutput(cmdutil.Options{JSONOutput: true}, nil, filepath.Join(dir, "missing"), "dest")
	check(t, err != nil, "expected output staging error")
	check(t, checkDownloadDestinationWritable(dir, true) != nil, "expected directory error")
	check(t, checkDownloadDestinationWritable("review/", false) != nil, "expected trailing-separator error")
	t.Chdir(t.TempDir())
	must(t, os.WriteFile(reviewDirectoryName, nil, 0o600))
	_, err = downloadDestination("", "id")
	check(t, err != nil, "expected review directory error")
}
func TestFilesDownloadStalledStorageResponseTimesOut(t *testing.T) {
	t.Chdir(t.TempDir())
	prev := downloadStallTimeout
	downloadStallTimeout = 150 * time.Millisecond
	t.Cleanup(func() { downloadStallTimeout = prev })
	release := make(chan struct{})
	storage := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-release
	})
	t.Cleanup(func() { close(release) })
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed?X-Amz-Signature=do-not-log"))
	})
	done := make(chan error, 1)
	go func() { done <- executeDownload([]string{"abc123", "f_1"}) }()
	select {
	case err := <-done:
		check(t, err != nil && strings.Contains(err.Error(), "sent no data"), "expected stall-timeout error, got: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("download blocked past the stall timeout")
	}
	_, err := os.Stat(implicitDownloadPath())
	check(t, err != nil, "stalled download must not install a destination file")
}
func TestFilesDownloadDoesNotCountSlowOutputAsStorageStall(t *testing.T) {
	t.Chdir(t.TempDir())
	previous := downloadStallTimeout
	downloadStallTimeout = 250 * time.Millisecond
	t.Cleanup(func() { downloadStallTimeout = previous })
	storage, _ := storageServer(t, strings.Repeat("x", 64*1024))
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	out := &slowFirstWriter{delay: 3 * downloadStallTimeout}
	must(t, executeDownload([]string{"abc123", "f_1", "-o", "-"}, testutil.Stdout(out)))
}
func TestFilesDownloadJSONStillWritesTheFile(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, _ := storageServer(t, "json mode bytes")
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed?X-Amz-Signature=do-not-log", func(p map[string]any) {
			p["file"].(map[string]any)["future_field"] = "preserved"
		}))
	})
	out := testutil.CaptureStdout(func() { must(t, executeDownload([]string{"abc123", "f_1"}, testutil.JSONOutput())) })
	data, err := os.ReadFile(implicitDownloadPath())
	check(t, err == nil, "JSON mode must still write the file: %v", err)
	check(t, string(data) == "json mode bytes", "unexpected file contents: %q", data)
	check(t, !strings.Contains(out, `"signed_url"`) && !strings.Contains(out, "do-not-log"), "JSON output leaked the signed URL: %q", out)
	check(t, strings.Contains(out, `"path": `+strconv.Quote(implicitDownloadPath())) && strings.Count(out, `"success"`) == 1, "expected one safe JSON result on stdout: %q", out)
	check(t, strings.Contains(out, `"deleted_at": null`) && strings.Contains(out, `"future_field": "preserved"`), "JSON output changed the raw file object: %q", out)
}
func TestFilesDownloadJQRuntimeErrorDoesNotDownload(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, reached := storageServer(t, "payload")
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	err := executeDownload([]string{"abc123", "f_1"}, testutil.JQ(`error("stop")`))
	check(t, err != nil && strings.Contains(err.Error(), "stop"), "expected jq runtime error, got: %v", err)
	check(t, !*reached, "storage was fetched after jq output failed")
	_, err = os.Stat(implicitDownloadPath())
	check(t, err != nil, "jq output failure must not write the file")
}
func TestFilesDownloadRejectsJSONWithStdoutDestination(t *testing.T) {
	storage, reached := storageServer(t, "payload")
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	err := executeDownload([]string{"abc123", "f_1", "-o", "-"}, testutil.JSONOutput())
	check(t, err != nil && strings.Contains(err.Error(), "cannot be combined with -o -"), "expected json/stdout conflict error, got: %v", err)
	check(t, !*reached, "storage was fetched despite the invalid flag combination")
}
func TestFilesDownloadStreamsToStdout(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, reached := storageServer(t, "streamed bytes")
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	var out bytes.Buffer
	must(t, executeDownload([]string{"abc123", "f_1", "-o", "-"}, testutil.Stdout(&out)))
	check(t, *reached && out.String() == "streamed bytes", "download request changed content encoding or bytes: %q", out.String())
	_, err := os.Stat(implicitDownloadPath())
	check(t, err != nil, "stdout mode must not leave a file behind")
}
func TestFilesDownloadExternalLinkPrintsTheLinkInsteadOfFetching(t *testing.T) {
	link := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {})
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(link.URL+"/page", func(p map[string]any) {
			p["external_link"] = true
		}))
	})
	err := executeDownload([]string{"abc123", "f_1", "-o", "occupied/"})
	check(t, err != nil && strings.Contains(err.Error(), "external link") && strings.Contains(err.Error(), link.URL+"/page"), "expected external-link error naming the URL, got: %v", err)
	err = executeDownload([]string{"abc123", "f_1", "-o", "-"}, testutil.JSONOutput())
	check(t, err != nil && strings.Contains(err.Error(), "external link"), "expected external-link error before JSON/stdout validation, got: %v", err)
}
func TestFilesDownloadRejectsInsecureStorageURL(t *testing.T) {
	reached := false
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	t.Cleanup(storage.Close)
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})
	err := executeDownload([]string{"abc123", "f_1"})
	check(t, err != nil && strings.Contains(err.Error(), "invalid secure download URL"), "expected secure-URL error, got: %v", err)
	check(t, !reached, "insecure storage URL was fetched")
}
func TestFilesDownloadDoesNotFollowStorageRedirects(t *testing.T) {
	target := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {})
	redirect := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusFound)
	})
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(redirect.URL+"/signed?X-Amz-Signature=secret"))
	})
	err := executeDownload([]string{"abc123", "f_1"})
	check(t, err != nil && strings.Contains(err.Error(), "HTTP 302"), "expected redirect refusal, got: %v", err)
	check(t, !strings.Contains(err.Error(), "secret"), "redirect error leaked the signed URL: %v", err)
}
func TestFilesDownloadTransportErrorRedactsSignedURL(t *testing.T) {
	storage := newDownloadTestServer(t, func(w http.ResponseWriter, r *http.Request) {})
	storage.Close()
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed?X-Amz-Signature=do-not-log"))
	})
	err := executeDownload([]string{"abc123", "f_1"})
	check(t, err != nil, "expected storage transport error")
	check(t, !strings.Contains(err.Error(), "do-not-log") && !strings.Contains(err.Error(), storage.URL), "transport error leaked the signed URL: %v", err)
}
