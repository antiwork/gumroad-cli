package products

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func downloadURLPayload(signedURL string, mutators ...func(map[string]any)) map[string]any {
	payload := map[string]any{
		"signed_url":    signedURL,
		"external_link": false,
		"file": map[string]any{
			"id":           "f_1",
			"display_name": "Big Guide",
			"file_name":    "big-guide.pdf",
			"extension":    "PDF",
			"filegroup":    "document",
			"file_size":    11,
			"created_at":   "2026-05-01T12:00:00Z",
			"deleted_at":   nil,
		},
	}
	for _, mutate := range mutators {
		mutate(payload)
	}
	return payload
}

func newHTTPTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func storageServer(t *testing.T, body string) (*httptest.Server, *bool) {
	t.Helper()
	reached := new(bool)
	srv := newHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write storage body: %v", err)
		}
	})
	return srv, reached
}

func TestFilesDownloadWritesImplicitDestinationFromFileName(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, _ := storageServer(t, "hello bytes")

	var gotPath, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})

	cmd := testutil.Command(newFilesDownloadCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"abc123", "f_1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/internal/admin/products/abc123/files/f_1/download_url" {
		t.Fatalf("unexpected admin path: %s", gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("unexpected auth header: %s", gotAuth)
	}
	data, err := os.ReadFile("big-guide.pdf")
	if err != nil {
		t.Fatalf("expected big-guide.pdf to be written: %v", err)
	}
	if string(data) != "hello bytes" {
		t.Fatalf("unexpected file contents: %q", data)
	}
	if !strings.Contains(out, "Downloaded Big Guide → big-guide.pdf") {
		t.Fatalf("missing success line in output: %q", out)
	}
	assertNoTempFiles(t)
}

func TestFilesDownloadImplicitDestinationCannotEscapeCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	storage, _ := storageServer(t, "payload")

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed", func(p map[string]any) {
			p["file"].(map[string]any)["file_name"] = "../evil.pdf"
		}))
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1"})
	testutil.MustExecute(t, cmd)

	if _, err := os.Stat(filepath.Join(dir, "..", "evil.pdf")); err == nil {
		t.Fatal("file escaped the working directory")
	}
	if _, err := os.Stat("evil.pdf"); err != nil {
		t.Fatalf("expected base-named file in cwd: %v", err)
	}
	assertNoTempFiles(t)
}

func TestFilesDownloadImplicitDestinationStripsControlCharacters(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, _ := storageServer(t, "payload")

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed", func(p map[string]any) {
			p["file"].(map[string]any)["file_name"] = "evil\x1b[31m\nname.pdf"
			p["file"].(map[string]any)["display_name"] = "Evil\nPack\x1b[31m"
		}))
	})

	var out bytes.Buffer
	cmd := testutil.Command(newFilesDownloadCmd(), testutil.Quiet(false), testutil.Stdout(&out))
	cmd.SetArgs([]string{"abc123", "f_1"})
	testutil.MustExecute(t, cmd)

	if _, err := os.Stat("evil[31mname.pdf"); err != nil {
		t.Fatalf("expected control-stripped filename in cwd: %v", err)
	}
	rendered := out.String()
	if strings.Contains(rendered, "\x1b[31m") {
		t.Fatalf("raw ANSI escape reached stdout: %q", rendered)
	}
	if !strings.Contains(rendered, `Evil\nPack\x1b[31m`) {
		t.Fatalf("expected escaped display name in output: %q", rendered)
	}
}

func TestFilesDownloadRefusesExistingDestinationBeforeRequest(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("review.zip", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	reached := false
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1", "-o", "review.zip"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got: %v", err)
	}
	if reached {
		t.Fatal("admin API was called despite the pre-request guard")
	}
	data, _ := os.ReadFile("review.zip")
	if string(data) != "old" {
		t.Fatalf("existing file was modified: %q", data)
	}
}

func TestFilesDownloadRefusesDestinationAppearingMidDownload(t *testing.T) {
	t.Chdir(t.TempDir())
	storage := newHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// The destination appears while the download is in flight; --force
		// was not given, so the replace-time check must refuse.
		if err := os.WriteFile("big-guide.pdf", []byte("raced"), 0o600); err != nil {
			t.Errorf("seed raced destination: %v", err)
		}
		_, _ = w.Write([]byte("new bytes"))
	})

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got: %v", err)
	}
	data, _ := os.ReadFile("big-guide.pdf")
	if string(data) != "raced" {
		t.Fatalf("raced file was overwritten: %q", data)
	}
	assertNoTempFiles(t)
}

func TestFilesDownloadForceOverwritesDestination(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("review.zip", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage, _ := storageServer(t, "fresh")

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1", "-o", "review.zip", "--force"})
	testutil.MustExecute(t, cmd)

	data, _ := os.ReadFile("review.zip")
	if string(data) != "fresh" {
		t.Fatalf("expected overwritten contents, got: %q", data)
	}
}

func TestFilesDownloadJSONStillWritesTheFile(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, _ := storageServer(t, "json mode bytes")

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})

	cmd := testutil.Command(newFilesDownloadCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"abc123", "f_1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	data, err := os.ReadFile("big-guide.pdf")
	if err != nil {
		t.Fatalf("JSON mode must still write the file: %v", err)
	}
	if string(data) != "json mode bytes" {
		t.Fatalf("unexpected file contents: %q", data)
	}
	if !strings.Contains(out, `"signed_url"`) || strings.Count(out, `"success"`) != 1 {
		t.Fatalf("expected exactly one JSON envelope on stdout: %q", out)
	}
}

func TestFilesDownloadRejectsJSONWithStdoutDestination(t *testing.T) {
	reached := false
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	cmd := testutil.Command(newFilesDownloadCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"abc123", "f_1", "-o", "-"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined with -o -") {
		t.Fatalf("expected json/stdout conflict error, got: %v", err)
	}
	if reached {
		t.Fatal("admin API was called despite the invalid flag combination")
	}
}

func TestFilesDownloadStreamsToStdout(t *testing.T) {
	t.Chdir(t.TempDir())
	storage, _ := storageServer(t, "streamed bytes")

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})

	var out bytes.Buffer
	cmd := testutil.Command(newFilesDownloadCmd(), testutil.Stdout(&out))
	cmd.SetArgs([]string{"abc123", "f_1", "-o", "-"})
	testutil.MustExecute(t, cmd)

	if out.String() != "streamed bytes" {
		t.Fatalf("unexpected stdout stream: %q", out.String())
	}
	if _, err := os.Stat("big-guide.pdf"); err == nil {
		t.Fatal("stdout mode must not leave a file behind")
	}
}

func TestFilesDownloadExternalLinkPrintsTheLinkInsteadOfFetching(t *testing.T) {
	t.Chdir(t.TempDir())
	linkFetched := false
	link := newHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		linkFetched = true
	})

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(link.URL+"/page", func(p map[string]any) {
			p["external_link"] = true
		}))
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "external link") || !strings.Contains(err.Error(), link.URL+"/page") {
		t.Fatalf("expected external-link error naming the URL, got: %v", err)
	}
	if linkFetched {
		t.Fatal("external link must not be fetched")
	}
	assertNoTempFiles(t)
}

func TestFilesDownloadStorageErrorLeavesDestinationUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("review.zip", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage := newHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(storage.URL+"/signed"))
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1", "-o", "review.zip", "--force"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected storage HTTP error, got: %v", err)
	}
	data, _ := os.ReadFile("review.zip")
	if string(data) != "old" {
		t.Fatalf("destination was modified on a failed download: %q", data)
	}
	assertNoTempFiles(t)
}

func TestFilesDownloadBlankSignedURLFails(t *testing.T) {
	t.Chdir(t.TempDir())
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, downloadURLPayload(""))
	})

	cmd := testutil.Command(newFilesDownloadCmd())
	cmd.SetArgs([]string{"abc123", "f_1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "did not return a download URL") {
		t.Fatalf("expected blank signed_url error, got: %v", err)
	}
}

func assertNoTempFiles(t *testing.T) {
	t.Helper()
	leftovers, err := filepath.Glob(".gumroad-download-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) > 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}
