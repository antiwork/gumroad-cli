package products

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

type createUploadServers struct {
	s3 *httptest.Server

	mu               sync.Mutex
	presignFilenames []string
	productForm      url.Values
	s3Calls          int
	completeCalls    int
	presignCalls     int
}

func newCreateUploadServers(t *testing.T) *createUploadServers {
	t.Helper()

	srv := &createUploadServers{}
	srv.s3 = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.mu.Lock()
		srv.s3Calls++
		srv.mu.Unlock()

		if r.Method != http.MethodPut {
			t.Errorf("S3 got %s, want PUT", r.Method)
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.s3.Close)

	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = srv.s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

	return srv
}

func (srv *createUploadServers) dispatch(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		switch r.URL.Path {
		case "/files/presign":
			srv.mu.Lock()
			srv.presignCalls++
			n := srv.presignCalls
			srv.presignFilenames = append(srv.presignFilenames, r.PostForm.Get("filename"))
			srv.mu.Unlock()

			testutil.JSON(t, w, map[string]any{
				"upload_id": "up-" + strconv.Itoa(n),
				"key":       "attachments/u/k/original/" + strconv.Itoa(n) + ".bin",
				"file_url":  "https://example.com/uploads/up-" + strconv.Itoa(n),
				"parts": []map[string]any{
					{"part_number": 1, "presigned_url": srv.s3.URL + "/part/" + strconv.Itoa(n)},
				},
			})
		case "/files/complete":
			srv.mu.Lock()
			srv.completeCalls++
			srv.mu.Unlock()

			testutil.JSON(t, w, map[string]any{
				"file_url": "https://example.com/uploads/" + r.PostForm.Get("upload_id"),
			})
		case "/products":
			srv.mu.Lock()
			srv.productForm = cloneURLValues(r.PostForm)
			srv.mu.Unlock()

			testutil.JSON(t, w, map[string]any{
				"product": map[string]any{
					"id":              "prod-upload",
					"name":            r.PostForm.Get("name"),
					"formatted_price": "$10",
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

func (srv *createUploadServers) snapshot() ([]string, url.Values, int, int) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return append([]string(nil), srv.presignFilenames...), cloneURLValues(srv.productForm), srv.s3Calls, srv.completeCalls
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, current := range values {
		cloned[key] = append([]string(nil), current...)
	}
	return cloned
}

func writeCreateFixture(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestCreate_WithFiles_UploadsAndPostsIndexedFields(t *testing.T) {
	srv := newCreateUploadServers(t)
	testutil.Setup(t, srv.dispatch(t))

	firstPath := writeCreateFixture(t, "first")
	secondPath := writeCreateFixture(t, "second")

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{
		"--name", "Art Pack",
		"--price", "10.00",
		"--file", firstPath,
		"--file", secondPath,
		"--file-name", "Custom One.zip",
		"--file-name", "",
		"--file-description", "",
		"--file-description", "Second file",
	})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	presignFilenames, productForm, s3Calls, completeCalls := srv.snapshot()
	if !reflect.DeepEqual(presignFilenames, []string{"Custom One.zip", filepath.Base(secondPath)}) {
		t.Fatalf("presign filenames = %v", presignFilenames)
	}
	if got := productForm.Get("files[0][url]"); got != "https://example.com/uploads/up-1" {
		t.Fatalf("files[0][url] = %q", got)
	}
	if got := productForm.Get("files[0][display_name]"); got != "Custom One.zip" {
		t.Fatalf("files[0][display_name] = %q", got)
	}
	if got := productForm.Get("files[0][description]"); got != "" {
		t.Fatalf("files[0][description] = %q, want empty", got)
	}
	if got := productForm.Get("files[1][url]"); got != "https://example.com/uploads/up-2" {
		t.Fatalf("files[1][url] = %q", got)
	}
	if got := productForm.Get("files[1][display_name]"); got != "" {
		t.Fatalf("files[1][display_name] = %q, want empty", got)
	}
	if got := productForm.Get("files[1][description]"); got != "Second file" {
		t.Fatalf("files[1][description] = %q", got)
	}
	if s3Calls != 2 {
		t.Fatalf("S3 calls = %d, want 2", s3Calls)
	}
	if completeCalls != 2 {
		t.Fatalf("complete calls = %d, want 2", completeCalls)
	}
	if !strings.Contains(out, "Created draft product:") || !strings.Contains(out, "prod-upload") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCreate_WithFiles_DryRunPrintsUploadsAndPlaceholderRequest(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("dry run must not reach the API")
	})

	firstPath := writeCreateFixture(t, "first")
	secondPath := writeCreateFixture(t, "second")

	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{
		"--name", "Art Pack",
		"--file", firstPath,
		"--file", secondPath,
		"--file-name", "Cover.zip",
		"--file-name", "",
		"--file-description", "",
		"--file-description", "Second file",
	})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "Dry run: upload "+firstPath) {
		t.Fatalf("missing first upload plan: %q", out)
	}
	if !strings.Contains(out, "Dry run: upload "+secondPath) {
		t.Fatalf("missing second upload plan: %q", out)
	}
	if !strings.Contains(out, "Dry run: POST /products") {
		t.Fatalf("missing create request: %q", out)
	}
	if !strings.Contains(out, "files[0][url]: <uploaded:file:0>") {
		t.Fatalf("missing first placeholder URL: %q", out)
	}
	if !strings.Contains(out, "files[1][description]: Second file") {
		t.Fatalf("missing second description: %q", out)
	}
}

func TestCreate_WithFiles_DryRunJSONIncludesUploadsAndRequest(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("dry run must not reach the API")
	})

	path := writeCreateFixture(t, "payload")

	cmd := testutil.Command(newCreateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{
		"--name", "Art Pack",
		"--file", path,
		"--file-name", "Gift.zip",
		"--file-description", "Bonus download",
	})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var payload dryRunCreatePayload
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if !payload.DryRun {
		t.Fatalf("expected dry_run=true, got %+v", payload)
	}
	if len(payload.Uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(payload.Uploads))
	}
	if got := payload.Uploads[0].Filename; got != "Gift.zip" {
		t.Fatalf("upload filename = %q", got)
	}
	if payload.Request.Method != "POST" || payload.Request.Path != "/products" {
		t.Fatalf("request = %+v", payload.Request)
	}
	if got := payload.Request.Params.Get("files[0][url]"); got != "<uploaded:file:0>" {
		t.Fatalf("files[0][url] = %q", got)
	}
	if got := payload.Request.Params.Get("files[0][display_name]"); got != "Gift.zip" {
		t.Fatalf("files[0][display_name] = %q", got)
	}
	if got := payload.Request.Params.Get("files[0][description]"); got != "Bonus download" {
		t.Fatalf("files[0][description] = %q", got)
	}
}

func TestCreate_FileMetadataCountMustMatchFiles(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("validation errors must not reach the API")
	})

	firstPath := writeCreateFixture(t, "first")
	secondPath := writeCreateFixture(t, "second")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "file name count mismatch",
			args: []string{
				"--name", "Art Pack",
				"--file", firstPath,
				"--file", secondPath,
				"--file-name", "Only one name",
			},
			want: "--file-name must be provided zero times or exactly once per --file",
		},
		{
			name: "file description count mismatch",
			args: []string{
				"--name", "Art Pack",
				"--file", firstPath,
				"--file", secondPath,
				"--file-description", "Only one description",
			},
			want: "--file-description must be provided zero times or exactly once per --file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := testutil.Command(newCreateCmd())
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
