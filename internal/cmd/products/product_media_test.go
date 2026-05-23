package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

type productMediaServers struct {
	direct *httptest.Server

	mu                sync.Mutex
	apiSequence       []string
	productCreateForm []map[string]string
	directUploadForm  []map[string]string
	directPUTHeaders  []http.Header
	attachSignedIDs   []string
	productPUTCalls   int
}

func newProductMediaServers(t *testing.T) *productMediaServers {
	t.Helper()

	s := &productMediaServers{}
	s.direct = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("direct upload got %s, want PUT", r.Method)
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		s.mu.Lock()
		s.directPUTHeaders = append(s.directPUTHeaders, r.Header.Clone())
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.direct.Close)
	return s
}

func (s *productMediaServers) dispatch(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.apiSequence = append(s.apiSequence, r.Method+" "+r.URL.Path)
		s.mu.Unlock()

		switch r.URL.Path {
		case "/products":
			if r.Method != http.MethodPost {
				t.Errorf("/products got %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			s.mu.Lock()
			s.productCreateForm = append(s.productCreateForm, map[string]string{
				"content_type": r.Header.Get("Content-Type"),
				"name":         r.PostForm.Get("name"),
				"price":        r.PostForm.Get("price"),
				"native_type":  r.PostForm.Get("native_type"),
			})
			s.mu.Unlock()
			testutil.JSON(t, w, map[string]any{
				"product": map[string]any{
					"id":              "prod-media",
					"name":            "Art Pack",
					"formatted_price": "$10",
				},
			})
		case "/products/prod1":
			if r.Method != http.MethodPut {
				t.Errorf("/products/prod1 got %s, want PUT", r.Method)
			}
			s.mu.Lock()
			s.productPUTCalls++
			s.mu.Unlock()
			testutil.JSON(t, w, map[string]any{"success": true})
		case "/direct_uploads":
			if r.Method != http.MethodPost {
				t.Errorf("/direct_uploads got %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			s.mu.Lock()
			n := len(s.directUploadForm) + 1
			form := map[string]string{
				"filename":     r.PostForm.Get("blob[filename]"),
				"byte_size":    r.PostForm.Get("blob[byte_size]"),
				"checksum":     r.PostForm.Get("blob[checksum]"),
				"content_type": r.PostForm.Get("blob[content_type]"),
			}
			s.directUploadForm = append(s.directUploadForm, form)
			s.mu.Unlock()
			testutil.JSON(t, w, map[string]any{
				"signed_id":    "signed-" + strconv.Itoa(n),
				"filename":     form["filename"],
				"byte_size":    form["byte_size"],
				"checksum":     form["checksum"],
				"content_type": form["content_type"],
				"direct_upload": map[string]any{
					"url": s.direct.URL + "/upload/" + strconv.Itoa(n),
					"headers": map[string]string{
						"Content-Type": form["content_type"],
						"Content-MD5":  form["checksum"],
					},
				},
			})
		case "/products/prod-media/covers", "/products/prod-media/thumbnail", "/products/prod1/covers", "/products/prod1/thumbnail", "/products/prod1/covers/cover-1":
			if r.Method == http.MethodDelete {
				testutil.JSON(t, w, map[string]any{"success": true})
				return
			}
			if r.Method != http.MethodPost {
				t.Errorf("%s got %s, want POST or DELETE", r.URL.Path, r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			s.mu.Lock()
			s.attachSignedIDs = append(s.attachSignedIDs, r.PostForm.Get("signed_blob_id"))
			s.mu.Unlock()
			switch {
			case strings.HasSuffix(r.URL.Path, "/thumbnail"):
				testutil.JSON(t, w, map[string]any{
					"success":   true,
					"thumbnail": map[string]any{"guid": "thumb-1"},
				})
			default:
				testutil.JSON(t, w, map[string]any{
					"success":       true,
					"covers":        []map[string]any{{"id": "cover-1"}},
					"main_cover_id": "cover-1",
				})
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

func (s *productMediaServers) snapshot() ([]string, []map[string]string, []http.Header, []string, int, []map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.apiSequence...),
		append([]map[string]string(nil), s.directUploadForm...),
		append([]http.Header(nil), s.directPUTHeaders...),
		append([]string(nil), s.attachSignedIDs...),
		s.productPUTCalls,
		append([]map[string]string(nil), s.productCreateForm...)
}

func writeMediaFixture(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func jpegFixtureContents() string {
	return string([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01})
}

func pngFixtureContents() string {
	return string([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0d})
}

func gifFixtureContents() string {
	return "GIF89a0000000000"
}

func webPFixtureContents() string {
	return string([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '})
}

func TestCreate_WithCoverAndThumbnail_CreatesThenUploadsAndAttachesMedia(t *testing.T) {
	srv := newProductMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	coverPath := writeMediaFixture(t, "cover.jpg", jpegFixtureContents())
	thumbPath := writeMediaFixture(t, "thumb.png", pngFixtureContents())

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{
		"--name", "Art Pack",
		"--price", "10.00",
		"--cover-image", coverPath,
		"--thumbnail", thumbPath,
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	sequence, forms, puts, signedIDs, _, productCreateForms := srv.snapshot()
	wantSequence := []string{
		"POST /products",
		"POST /direct_uploads",
		"POST /products/prod-media/covers",
		"POST /direct_uploads",
		"POST /products/prod-media/thumbnail",
	}
	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Fatalf("API sequence = %#v, want %#v", sequence, wantSequence)
	}
	if len(forms) != 2 {
		t.Fatalf("direct uploads = %d, want 2", len(forms))
	}
	if forms[0]["filename"] != "cover.jpg" || forms[0]["content_type"] != "image/jpeg" {
		t.Fatalf("cover direct upload form = %#v", forms[0])
	}
	if forms[1]["filename"] != "thumb.png" || forms[1]["content_type"] != "image/png" {
		t.Fatalf("thumbnail direct upload form = %#v", forms[1])
	}
	if len(puts) != 2 || puts[0].Get("Content-MD5") == "" || puts[1].Get("Content-MD5") == "" {
		t.Fatalf("direct upload PUT headers = %#v", puts)
	}
	if !reflect.DeepEqual(signedIDs, []string{"signed-1", "signed-2"}) {
		t.Fatalf("attached signed IDs = %#v", signedIDs)
	}
	if len(productCreateForms) != 1 {
		t.Fatalf("product create forms = %d, want 1", len(productCreateForms))
	}
	if !strings.HasPrefix(productCreateForms[0]["content_type"], "application/x-www-form-urlencoded") {
		t.Fatalf("product create content type = %q, want form encoded", productCreateForms[0]["content_type"])
	}
	if productCreateForms[0]["name"] != "Art Pack" || productCreateForms[0]["price"] != "1000" || productCreateForms[0]["native_type"] != "digital" {
		t.Fatalf("product create form = %#v", productCreateForms[0])
	}

	var payload struct {
		Product struct {
			ID string `json:"id"`
		} `json:"product"`
		Media []productMediaAttachmentResult `json:"media"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse JSON output: %v\n%s", err, out)
	}
	if payload.Product.ID != "prod-media" || len(payload.Media) != 2 {
		t.Fatalf("unexpected output payload: %+v", payload)
	}
}

func TestMergeProductMediaResultPreservesRawResponseFormatting(t *testing.T) {
	data := json.RawMessage(`{"product":{"id":"prod-media","rank":1.0}}`)
	media := []productMediaAttachmentResult{{
		Kind:     "cover",
		Path:     "cover.jpg",
		Endpoint: "/products/prod-media/covers",
		Response: json.RawMessage(`{"success":true}`),
	}}

	merged, err := mergeProductMediaResult(data, media)
	if err != nil {
		t.Fatalf("mergeProductMediaResult: %v", err)
	}
	if !json.Valid(merged) {
		t.Fatalf("merged result is not valid JSON: %s", merged)
	}
	if !strings.Contains(string(merged), `"rank":1.0`) {
		t.Fatalf("expected raw numeric formatting to be preserved, got:\n%s", merged)
	}
	if !strings.Contains(string(merged), `"media":[`) {
		t.Fatalf("expected media to be appended, got:\n%s", merged)
	}
}

func TestUpdate_WithPreviewImageOnly_DoesNotPutProduct(t *testing.T) {
	srv := newProductMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	previewPath := writeMediaFixture(t, "preview.gif", gifFixtureContents())
	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--preview-image", previewPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	sequence, forms, _, signedIDs, productPUTCalls, _ := srv.snapshot()
	wantSequence := []string{
		"POST /direct_uploads",
		"POST /products/prod1/covers",
	}
	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Fatalf("API sequence = %#v, want %#v", sequence, wantSequence)
	}
	if productPUTCalls != 0 {
		t.Fatalf("product PUT calls = %d, want 0", productPUTCalls)
	}
	if len(forms) != 1 || forms[0]["content_type"] != "image/gif" {
		t.Fatalf("direct upload form = %#v", forms)
	}
	if !reflect.DeepEqual(signedIDs, []string{"signed-1"}) {
		t.Fatalf("attached signed IDs = %#v", signedIDs)
	}
	var payload struct {
		Result struct {
			Success bool `json:"success"`
			Product struct {
				ID string `json:"id"`
			} `json:"product"`
			Media []productMediaAttachmentResult `json:"media"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse JSON output: %v\n%s", err, out)
	}
	if !payload.Result.Success || payload.Result.Product.ID != "prod1" || len(payload.Result.Media) != 1 {
		t.Fatalf("unexpected JSON output: %+v", payload)
	}
}

func TestUpdate_WithPreviewImageOnlyFailureDoesNotClaimProductUpdateCompleted(t *testing.T) {
	var sequence []string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/products/prod1" {
			t.Fatal("media-only update must not PUT product fields")
		}
		http.Error(w, "direct upload unavailable", http.StatusBadGateway)
	})

	previewPath := writeMediaFixture(t, "preview.gif", gifFixtureContents())
	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--preview-image", previewPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected direct upload error")
	}
	if strings.Contains(err.Error(), "product update completed") {
		t.Fatalf("unexpected completed product update context: %v", err)
	}
	if !reflect.DeepEqual(sequence, []string{"POST /direct_uploads"}) {
		t.Fatalf("API sequence = %#v", sequence)
	}
}

func TestCreate_WithMultipleMediaFailureShowsRetryForFailedAndSkippedMedia(t *testing.T) {
	var sequence []string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/products":
			testutil.JSON(t, w, map[string]any{
				"product": map[string]any{"id": "prod-media"},
			})
		case "/direct_uploads":
			http.Error(w, "direct upload unavailable", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	coverPath := writeMediaFixture(t, "cover.jpg", jpegFixtureContents())
	thumbPath := writeMediaFixture(t, "thumb.png", pngFixtureContents())
	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{
		"--name", "Art Pack",
		"--cover-image", coverPath,
		"--thumbnail", thumbPath,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected direct upload error")
	}
	for _, want := range []string{
		"product create completed for product prod-media",
		"retry remaining media with:",
		"gumroad products covers add prod-media --image",
		"gumroad products thumbnail set prod-media --image",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in error: %v", want, err)
		}
	}
	if !reflect.DeepEqual(sequence, []string{"POST /products", "POST /direct_uploads"}) {
		t.Fatalf("API sequence = %#v", sequence)
	}
}

func TestUpdate_WithPreviewImageDryRunJSONShowsDirectUploadAndAttachRequests(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("dry run must not reach the API")
	})

	previewPath := writeMediaFixture(t, "preview.jpg", jpegFixtureContents())
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--preview-image", previewPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var payload dryRunUpdateBody
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse JSON dry-run output: %v\n%s", err, out)
	}
	if len(payload.Uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(payload.Uploads))
	}
	if payload.Uploads[0].Action != "direct_upload" || payload.Uploads[0].Kind != "preview" || payload.Uploads[0].ContentType != "image/jpeg" {
		t.Fatalf("unexpected upload plan: %+v", payload.Uploads[0])
	}
	if len(payload.Preserved) != 0 || len(payload.Removed) != 0 {
		t.Fatalf("unexpected file update delta: preserved=%+v removed=%+v", payload.Preserved, payload.Removed)
	}
	requests := append([]dryRunCreateRequest{payload.Request}, payload.FollowUpRequests...)
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if requests[0].Path != "/direct_uploads" || requests[1].Path != "/products/prod1/covers" {
		t.Fatalf("unexpected requests: %+v", requests)
	}
}

func TestCreate_WithMediaDryRunPlainAndHumanShowsDirectUploadFlow(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("dry run must not reach the API")
	})

	for _, tc := range []struct {
		name     string
		mutators []testutil.OptionsMutator
		want     []string
	}{
		{
			name:     "plain",
			mutators: []testutil.OptionsMutator{testutil.DryRun(true), testutil.PlainOutput()},
			want:     []string{"direct_upload", "POST\t/direct_uploads", "POST\t/products/created-product-id/covers"},
		},
		{
			name:     "human",
			mutators: []testutil.OptionsMutator{testutil.DryRun(true)},
			want:     []string{"Dry run: direct upload", "Content type: image/jpeg", "Dry run: POST /direct_uploads", "Dry run: POST /products/created-product-id/covers"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeMediaFixture(t, "cover.jpg", jpegFixtureContents())
			cmd := testutil.Command(newCreateCmd(), tc.mutators...)
			cmd.SetArgs([]string{"--name", "Art Pack", "--cover-image", path})
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("expected %q in output:\n%s", want, out)
				}
			}
		})
	}
}

func TestProductMediaRejectsWebPClientSide(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("unsupported media must not reach the API")
	})

	path := writeMediaFixture(t, "cover.webp", "webp bytes")
	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--name", "Art Pack", "--cover-image", path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected WebP validation error")
	}
	if !strings.Contains(err.Error(), "WebP images are not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProductMediaRejectsRenamedWebPClientSide(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("unsupported media must not reach the API")
	})

	path := writeMediaFixture(t, "cover.jpg", webPFixtureContents())
	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--name", "Art Pack", "--cover-image", path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected WebP validation error")
	}
	if !strings.Contains(err.Error(), "WebP images are not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProductMediaRejectsRenamedNonImageClientSide(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("unsupported media must not reach the API")
	})

	path := writeMediaFixture(t, "cover.jpg", "%PDF-1.7\nnot an image")
	cmd := testutil.Command(newCreateCmd())
	cmd.SetArgs([]string{"--name", "Art Pack", "--cover-image", path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-image validation error")
	}
	if !strings.Contains(err.Error(), "unsupported product media type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProductMediaRejectsOversizedImagesClientSide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.jpg")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if err := file.Truncate(uploadMaxProductMediaFileSize() + 1); err != nil {
		t.Fatalf("truncate fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}

	_, err = describeSingleProductMedia(requestedProductMedia{Kind: productMediaCover, Path: path})
	if err == nil {
		t.Fatal("expected image size validation error")
	}
	if !strings.Contains(err.Error(), "50 MB") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProductMediaPlanningAndRetryHelpers(t *testing.T) {
	collected := collectProductMedia("cover.jpg", []string{"preview-a.jpg", "preview-b.jpg"}, "thumb.jpg")
	if len(collected) != 4 || collected[0].Kind != productMediaCover || collected[3].Kind != productMediaThumbnail {
		t.Fatalf("unexpected collected media: %+v", collected)
	}

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "Art Pack", "--preview-image", ""})
	if err := cmd.ParseFlags([]string{"--preview-image", ""}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := validateProductMediaFlagPaths(cmd, "", []string{""}, ""); err == nil || !strings.Contains(err.Error(), "--preview-image cannot be empty") {
		t.Fatalf("expected empty preview-image error, got %v", err)
	}

	if got := productMediaRetryCommand("prod1", plannedProductMedia{requestedProductMedia: requestedProductMedia{Kind: productMediaThumbnail, Path: "thumb one.jpg"}}); got != "gumroad products thumbnail set prod1 --image 'thumb one.jpg'" {
		t.Fatalf("thumbnail retry command = %q", got)
	}
	if got := productMediaRetryCommand("prod1", plannedProductMedia{requestedProductMedia: requestedProductMedia{Kind: productMediaCover, Path: "cover.jpg"}}); got != "gumroad products covers add prod1 --image cover.jpg" {
		t.Fatalf("cover retry command = %q", got)
	}
	wrapped := wrapProductMediaAttachError(fmt.Errorf("upload failed"), "prod1", "product create", false, []plannedProductMedia{
		{requestedProductMedia: requestedProductMedia{Kind: productMediaCover, Path: "cover.jpg"}},
		{requestedProductMedia: requestedProductMedia{Kind: productMediaThumbnail, Path: "thumb.jpg"}},
	})
	if !strings.Contains(wrapped.Error(), "product create completed for product prod1") {
		t.Fatalf("wrapped error did not include retry context: %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "gumroad products thumbnail set prod1 --image thumb.jpg") {
		t.Fatalf("wrapped error did not include skipped media retry context: %v", wrapped)
	}
}

func TestDetectProductImageContentTypeSniffsExtensionlessImages(t *testing.T) {
	path := writeMediaFixture(t, "cover", gifFixtureContents())
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = file.Close() }()

	contentType, err := detectProductImageContentType(path, file)
	if err != nil {
		t.Fatalf("detectProductImageContentType: %v", err)
	}
	if contentType != "image/gif" {
		t.Fatalf("content type = %q, want image/gif", contentType)
	}
}

func TestDirectUploadProductMediaRejectsIncompleteServerResponses(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/direct_uploads" {
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"direct_upload": map[string]any{"url": "https://example.com/upload"},
		})
	})

	path := writeMediaFixture(t, "cover.jpg", jpegFixtureContents())
	media, err := describeSingleProductMedia(requestedProductMedia{Kind: productMediaCover, Path: path})
	if err != nil {
		t.Fatalf("describeSingleProductMedia: %v", err)
	}
	_, err = directUploadProductMedia(testutil.TestOptions(), testutilClient(t), media)
	if err == nil || !strings.Contains(err.Error(), "signed_id") {
		t.Fatalf("expected missing signed_id error, got %v", err)
	}
}

func TestPutDirectUploadReportsServerBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "storage unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	path := writeMediaFixture(t, "cover.jpg", jpegFixtureContents())
	media, err := describeSingleProductMedia(requestedProductMedia{Kind: productMediaCover, Path: path})
	if err != nil {
		t.Fatalf("describeSingleProductMedia: %v", err)
	}
	err = putDirectUpload(testutil.TestOptions(), media, server.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "storage unavailable") {
		t.Fatalf("expected direct upload server error, got %v", err)
	}
}

func TestCoversAdd_WithImageUploadsAndAttaches(t *testing.T) {
	srv := newProductMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeMediaFixture(t, "cover.jpg", jpegFixtureContents())
	cmd := testutil.Command(newCoversAddCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--image", path})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	sequence, forms, _, signedIDs, _, _ := srv.snapshot()
	if !reflect.DeepEqual(sequence, []string{"POST /direct_uploads", "POST /products/prod1/covers"}) {
		t.Fatalf("API sequence = %#v", sequence)
	}
	if len(forms) != 1 || forms[0]["filename"] != "cover.jpg" {
		t.Fatalf("direct upload form = %#v", forms)
	}
	if !reflect.DeepEqual(signedIDs, []string{"signed-1"}) {
		t.Fatalf("attached signed IDs = %#v", signedIDs)
	}
}

func TestCoversAdd_WithURLSendsURL(t *testing.T) {
	var gotForm urlValues
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/products/prod1/covers" {
			t.Fatalf("got %s %s, want POST /products/prod1/covers", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = urlValues(r.PostForm)
		testutil.JSON(t, w, map[string]any{"success": true, "covers": []map[string]any{{"id": "cover-url"}}})
	})

	cmd := testutil.Command(newCoversAddCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--url", "https://www.youtube.com/watch?v=qKebcV1jv3A"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotForm.Get("url") != "https://www.youtube.com/watch?v=qKebcV1jv3A" {
		t.Fatalf("url = %q", gotForm.Get("url"))
	}
}

func TestCoversAdd_WithEmptyImageReturnsUsageError(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("empty image must not reach the API")
	})

	cmd := testutil.Command(newCoversAddCmd())
	cmd.SetArgs([]string{"prod1", "--image", ""})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected empty image error")
	}
	if !strings.Contains(err.Error(), "--image cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCoversRemoveAndThumbnailRemove(t *testing.T) {
	srv := newProductMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	coverCmd := testutil.Command(newCoversRemoveCmd(), testutil.Yes(true), testutil.JSONOutput())
	coverCmd.SetArgs([]string{"prod1", "cover-1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, coverCmd) })

	thumbnailCmd := testutil.Command(newThumbnailRemoveCmd(), testutil.Yes(true), testutil.JSONOutput())
	thumbnailCmd.SetArgs([]string{"prod1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, thumbnailCmd) })

	sequence, _, _, _, _, _ := srv.snapshot()
	if !reflect.DeepEqual(sequence, []string{"DELETE /products/prod1/covers/cover-1", "DELETE /products/prod1/thumbnail"}) {
		t.Fatalf("API sequence = %#v", sequence)
	}
}

func TestThumbnailSet_WithImageUploadsAndAttaches(t *testing.T) {
	srv := newProductMediaServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeMediaFixture(t, "thumb.png", pngFixtureContents())
	cmd := testutil.Command(newThumbnailSetCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--image", path})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	sequence, forms, _, signedIDs, _, _ := srv.snapshot()
	if !reflect.DeepEqual(sequence, []string{"POST /direct_uploads", "POST /products/prod1/thumbnail"}) {
		t.Fatalf("API sequence = %#v", sequence)
	}
	if len(forms) != 1 || forms[0]["filename"] != "thumb.png" || forms[0]["content_type"] != "image/png" {
		t.Fatalf("direct upload form = %#v", forms)
	}
	if !reflect.DeepEqual(signedIDs, []string{"signed-1"}) {
		t.Fatalf("attached signed IDs = %#v", signedIDs)
	}
}

func TestThumbnailSet_WithEmptyImageReturnsUsageError(t *testing.T) {
	testutil.Setup(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("empty image must not reach the API")
	})

	cmd := testutil.Command(newThumbnailSetCmd())
	cmd.SetArgs([]string{"prod1", "--image", ""})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected empty image error")
	}
	if !strings.Contains(err.Error(), "--image cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCoversReorder_SendsCoverIDs(t *testing.T) {
	var gotForm urlValues
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/products/prod1" {
			t.Fatalf("got %s %s, want PUT /products/prod1", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = urlValues(r.PostForm)
		testutil.JSON(t, w, map[string]any{"success": true})
	})

	cmd := testutil.Command(newCoversReorderCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "cover_b", "cover_a"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !reflect.DeepEqual(gotForm["cover_ids[]"], []string{"cover_b", "cover_a"}) {
		t.Fatalf("cover_ids[] = %#v", gotForm["cover_ids[]"])
	}
}

type urlValues map[string][]string

func (v urlValues) Get(key string) string {
	if len(v[key]) == 0 {
		return ""
	}
	return v[key][0]
}

func (v urlValues) String() string {
	return fmt.Sprint(map[string][]string(v))
}

func testutilClient(t *testing.T) *api.Client {
	t.Helper()
	token, err := config.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	return cmdutil.NewAPIClient(testutil.TestOptions(), token)
}
