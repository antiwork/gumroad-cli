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
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

type productUpdateFileServers struct {
	existingFiles                    []existingProductFile
	existingRichContent              []map[string]any
	hasSameRichContentForAllVariants bool
	variants                         []map[string]any

	s3 *httptest.Server

	getCalls             atomic.Int32
	putCalls             atomic.Int32
	jsonPutCalls         atomic.Int32
	s3Calls              atomic.Int32
	completeSeq          atomic.Int32
	failCompleteOn       int32
	putStatus            int
	rejectUnknownFileIDs bool

	putForm         url.Values
	putJSON         map[string]any
	putJSONResponse json.RawMessage
	presignBody     map[string]string
}

func newProductUpdateFileServers(t *testing.T) *productUpdateFileServers {
	t.Helper()

	s := &productUpdateFileServers{
		hasSameRichContentForAllVariants: true,
		presignBody:                      map[string]string{},
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
						"id":                                     "prod1",
						"files":                                  s.existingFiles,
						"rich_content":                           s.existingRichContent,
						"has_same_rich_content_for_all_variants": s.hasSameRichContentForAllVariants,
						"variants":                               s.variants,
					},
				})
			case http.MethodPut:
				s.putCalls.Add(1)
				if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					s.jsonPutCalls.Add(1)
					if err := json.NewDecoder(r.Body).Decode(&s.putJSON); err != nil {
						t.Fatalf("decode JSON body: %v", err)
					}
					if s.rejectUnknownFileIDs && s.rejectUnknownProductFileIDs(t, w) {
						return
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
				if len(s.putJSONResponse) > 0 {
					testutil.RawJSON(t, w, string(s.putJSONResponse))
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

func (s *productUpdateFileServers) rejectUnknownProductFileIDs(t *testing.T, w http.ResponseWriter) bool {
	t.Helper()

	files, ok := s.putJSON["files"].([]any)
	if !ok {
		return false
	}
	existing := make(map[string]struct{}, len(s.existingFiles))
	for _, file := range s.existingFiles {
		existing[file.ID] = struct{}{}
	}

	var missing []string
	for _, raw := range files {
		file, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("files entry has wrong type: %T", raw)
		}
		id, _ := file["id"].(string)
		if id == "" {
			continue
		}
		if _, ok := existing[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return false
	}

	testutil.RawJSON(t, w, fmt.Sprintf(
		`{"success":false,"message":"File(s) %s no longer exist; they may have been deleted by a concurrent request. Retry with the current file list."}`,
		strings.Join(missing, ", "),
	))
	return true
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

func productUpdateNewUploadExternalID(t *testing.T, file map[string]any, label string) string {
	t.Helper()

	if _, ok := file["id"]; ok {
		t.Fatalf("%s unexpectedly sent id: %#v", label, file)
	}
	externalID, ok := file["external_id"].(string)
	if !ok || !strings.HasPrefix(externalID, "cli-upload-") {
		t.Fatalf("%s.external_id = %#v, want generated cli upload id", label, file["external_id"])
	}
	return externalID
}

func productUpdateJSONRichContent(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	raw, ok := body["rich_content"].([]any)
	if !ok {
		t.Fatalf("rich_content payload has wrong type: %T", body["rich_content"])
	}
	pages := make([]map[string]any, len(raw))
	for i, current := range raw {
		page, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("rich_content[%d] has wrong type: %T", i, current)
		}
		pages[i] = page
	}
	return pages
}

func firstRichContentFileEmbedID(t *testing.T, page map[string]any) string {
	t.Helper()

	description, ok := page["description"].(map[string]any)
	if !ok {
		t.Fatalf("rich_content description has wrong type: %T", page["description"])
	}
	content, ok := description["content"].([]any)
	if !ok {
		t.Fatalf("rich_content content has wrong type: %T", description["content"])
	}
	for _, node := range content {
		nodeMap, ok := node.(map[string]any)
		if !ok || nodeMap["type"] != "fileEmbed" {
			continue
		}
		attrs, ok := nodeMap["attrs"].(map[string]any)
		if !ok {
			t.Fatalf("fileEmbed attrs has wrong type: %T", nodeMap["attrs"])
		}
		id, ok := attrs["id"].(string)
		if !ok || id == "" {
			t.Fatalf("fileEmbed id = %#v", attrs["id"])
		}
		return id
	}
	t.Fatalf("rich_content page has no fileEmbed: %#v", page)
	return ""
}

func richContentFileEmbedIDsFromBody(t *testing.T, body map[string]any) []string {
	t.Helper()

	richContent := productUpdateJSONRichContent(t, body)
	return fileEmbedIDs(richContent)
}

func firstRichContentNodeTypesFromBody(t *testing.T, body map[string]any) []string {
	t.Helper()

	richContent := productUpdateJSONRichContent(t, body)
	if len(richContent) == 0 {
		t.Fatal("rich_content payload is empty")
	}
	return richContentNodeTypes(t, richContent[0])
}

func richContentNodeTypes(t *testing.T, page map[string]any) []string {
	t.Helper()

	content := richContentPageContent(t, page)
	types := make([]string, len(content))
	for i, node := range content {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("rich_content content[%d] has wrong type: %T", i, node)
		}
		nodeType, ok := nodeMap["type"].(string)
		if !ok || nodeType == "" {
			t.Fatalf("rich_content content[%d].type = %#v", i, nodeMap["type"])
		}
		types[i] = nodeType
	}
	return types
}

func richContentPageContent(t *testing.T, page map[string]any) []any {
	t.Helper()

	description, ok := page["description"].(map[string]any)
	if !ok {
		t.Fatalf("rich_content description has wrong type: %T", page["description"])
	}
	content, ok := description["content"].([]any)
	if !ok {
		t.Fatalf("rich_content content has wrong type: %T", description["content"])
	}
	return content
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
	newFileID := productUpdateNewUploadExternalID(t, files[2], "files[2]")
	if files[2]["display_name"] != "New Pack.zip" {
		t.Fatalf("files[2].display_name = %#v", files[2]["display_name"])
	}
	if files[2]["description"] != "Updated bundle" {
		t.Fatalf("files[2].description = %#v", files[2]["description"])
	}
	if ids := richContentFileEmbedIDsFromBody(t, srv.putJSON); !reflect.DeepEqual(ids, []string{"file_a", "file_b", newFileID}) {
		t.Fatalf("rich_content fileEmbed ids = %#v, want preserved files and new upload", ids)
	}
}

func TestUpdate_WithFilesJSONPreservesRawProductResponseWithoutMedia(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.putJSONResponse = json.RawMessage(`{"success":true,"product":{"id":"prod1","rank":1.0}}`)
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--file", path})

	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, `"rank": 1.0`) {
		t.Fatalf("expected raw numeric formatting to be preserved, got:\n%s", out)
	}
	if strings.Contains(out, `"media"`) {
		t.Fatalf("file-only update should not add media output, got:\n%s", out)
	}
}

func TestUpdate_FileRejectsPerVariantContentWithoutAttachFlag(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.hasSameRichContentForAllVariants = false
	srv.variants = []map[string]any{{
		"title":   "Size",
		"options": []map[string]any{{"name": "Large"}},
	}}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "license bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "License.pdf",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected per-variant content error")
	}
	if !strings.Contains(err.Error(), "per-variant content") || !strings.Contains(err.Error(), "gumroad variants update") {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.s3Calls.Load() != 0 {
		t.Fatalf("S3 calls = %d, want 0", srv.s3Calls.Load())
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("PUT calls = %d, want 0", srv.putCalls.Load())
	}
}

func TestProductUsesPerVariantRichContentIsConservativeWhenVariantsOmitted(t *testing.T) {
	state := productFileUpdateState{
		HasSameRichContentForAllVariants: false,
	}
	if !productUsesPerVariantRichContent(state) {
		t.Fatal("expected omitted variants to be treated as per-variant content")
	}
}

func TestProductUsesPerVariantRichContentAllowsExplicitEmptyVariants(t *testing.T) {
	variants := []productVariantCategoryRef{}
	state := productFileUpdateState{
		HasSameRichContentForAllVariants: false,
		Variants:                         &variants,
	}
	if productUsesPerVariantRichContent(state) {
		t.Fatal("expected explicit empty variants to allow product-level content")
	}
}

func TestProductUsesPerVariantRichContentDetectsVariantOptions(t *testing.T) {
	variants := []productVariantCategoryRef{{
		Options: []productVariantOptionRef{{Name: "Large"}},
	}}
	state := productFileUpdateState{
		HasSameRichContentForAllVariants: false,
		Variants:                         &variants,
	}
	if !productUsesPerVariantRichContent(state) {
		t.Fatal("expected variant options to require variant-level content")
	}
}

func TestUpdate_FileCreatesRichContentForNewUpload(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 1 {
		t.Fatalf("files payload len = %d, want 1", len(files))
	}
	if got := files[0]["url"]; got != "https://example.com/attachments/u/k/original/upload-1.bin" {
		t.Fatalf("files[0].url = %#v", got)
	}
	externalID := productUpdateNewUploadExternalID(t, files[0], "files[0]")
	if ids := richContentFileEmbedIDsFromBody(t, srv.putJSON); !reflect.DeepEqual(ids, []string{externalID}) {
		t.Fatalf("rich_content fileEmbed ids = %#v, want new upload external_id", ids)
	}
}

func TestUpdate_FileUploadUsesExternalIDForNewFiles(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.rejectUnknownFileIDs = true
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 1 {
		t.Fatalf("files payload len = %d, want 1", len(files))
	}
	externalID := productUpdateNewUploadExternalID(t, files[0], "files[0]")
	if ids := richContentFileEmbedIDsFromBody(t, srv.putJSON); !reflect.DeepEqual(ids, []string{externalID}) {
		t.Fatalf("rich_content fileEmbed ids = %#v, want new upload external_id", ids)
	}
}

func TestUpdate_FileRollsExistingRichContentEmbed(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_old", Name: "Old Pack.zip"},
		{ID: "file_keep", Name: "Keep.pdf"},
	}
	srv.existingRichContent = []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{
					"type": "paragraph",
					"content": []any{
						map[string]any{"type": "text", "text": "Download below"},
					},
				},
				map[string]any{
					"type": "fileEmbed",
					"attrs": map[string]any{
						"id":        "file_old",
						"uid":       "old-uid",
						"collapsed": false,
					},
				},
			},
		},
	}}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "replacement bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 3 {
		t.Fatalf("files payload len = %d, want 3", len(files))
	}
	if got := files[0]["id"]; got != "file_old" {
		t.Fatalf("files[0].id = %#v, want file_old", got)
	}
	if got := files[1]["id"]; got != "file_keep" {
		t.Fatalf("files[1].id = %#v, want file_keep", got)
	}
	newFileID := productUpdateNewUploadExternalID(t, files[2], "files[2]")

	richContent := productUpdateJSONRichContent(t, srv.putJSON)
	if len(richContent) != 1 {
		t.Fatalf("rich_content len = %d, want 1", len(richContent))
	}
	if got := richContent[0]["id"]; got != "page_1" {
		t.Fatalf("rich_content page id = %#v, want page_1", got)
	}
	if got := firstRichContentFileEmbedID(t, richContent[0]); got != newFileID {
		t.Fatalf("fileEmbed id = %q, want new file id %q", got, newFileID)
	}
}

func TestUpdate_FileRollsBeforeTrailingParagraph(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_old", Name: "Old Pack.zip"},
	}
	srv.existingRichContent = []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_old"}},
				map[string]any{"type": "paragraph"},
			},
		},
	}}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 2 {
		t.Fatalf("files payload len = %d, want 2", len(files))
	}
	newFileID := productUpdateNewUploadExternalID(t, files[1], "files[1]")
	if ids := richContentFileEmbedIDsFromBody(t, srv.putJSON); !reflect.DeepEqual(ids, []string{newFileID}) {
		t.Fatalf("rich_content fileEmbed ids = %#v, want new upload only", ids)
	}
	if types := firstRichContentNodeTypesFromBody(t, srv.putJSON); !reflect.DeepEqual(types, []string{"fileEmbed", "paragraph"}) {
		t.Fatalf("rich_content node types = %#v, want file embed then one trailing paragraph", types)
	}
}

func TestUpdate_FileRollsOnPageWithExistingEmbed(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_old", Name: "Old Pack.zip"},
	}
	srv.existingRichContent = []map[string]any{
		{
			"id":    "page_1",
			"title": "Welcome",
			"description": map[string]any{
				"type": "doc",
				"content": []any{
					map[string]any{"type": "paragraph"},
					map[string]any{"type": "paragraph"},
				},
			},
		},
		{
			"id":    "page_2",
			"title": "Module 1",
			"description": map[string]any{
				"type": "doc",
				"content": []any{
					map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_old"}},
					map[string]any{"type": "paragraph"},
				},
			},
		},
	}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 2 {
		t.Fatalf("files payload len = %d, want 2", len(files))
	}
	newFileID := productUpdateNewUploadExternalID(t, files[1], "files[1]")

	richContent := productUpdateJSONRichContent(t, srv.putJSON)
	if len(richContent) != 2 {
		t.Fatalf("rich_content len = %d, want 2", len(richContent))
	}
	if ids := fileEmbedIDs([]map[string]any{richContent[0]}); len(ids) != 0 {
		t.Fatalf("page 1 fileEmbed ids = %#v, want none", ids)
	}
	if ids := fileEmbedIDs([]map[string]any{richContent[1]}); !reflect.DeepEqual(ids, []string{newFileID}) {
		t.Fatalf("page 2 fileEmbed ids = %#v, want new upload only", ids)
	}
	if types := richContentNodeTypes(t, richContent[0]); !reflect.DeepEqual(types, []string{"paragraph", "paragraph"}) {
		t.Fatalf("page 1 node types = %#v, want unchanged paragraphs", types)
	}
	if types := richContentNodeTypes(t, richContent[1]); !reflect.DeepEqual(types, []string{"fileEmbed", "paragraph"}) {
		t.Fatalf("page 2 node types = %#v, want file embed then one trailing paragraph", types)
	}
}

func TestUpdate_FileRollsMultipleEmbedsInsideExistingFileEmbedGroup(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a", Name: "Old A.zip"},
		{ID: "file_b", Name: "Old B.zip"},
	}
	srv.existingRichContent = []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{
					"type": "fileEmbedGroup",
					"content": []any{
						map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_a"}},
						map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_b"}},
					},
				},
				map[string]any{"type": "paragraph"},
			},
		},
	}}
	testutil.Setup(t, srv.dispatch(t))

	firstPath := writeProductUploadFixture(t, "first bytes")
	secondPath := writeProductUploadFixture(t, "second bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", firstPath,
		"--file", secondPath,
		"--file-name", "New A.zip",
		"--file-name", "New B.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 4 {
		t.Fatalf("files payload len = %d, want 4", len(files))
	}
	firstNewFileID := productUpdateNewUploadExternalID(t, files[2], "files[2]")
	secondNewFileID := productUpdateNewUploadExternalID(t, files[3], "files[3]")

	richContent := productUpdateJSONRichContent(t, srv.putJSON)
	if types := richContentNodeTypes(t, richContent[0]); !reflect.DeepEqual(types, []string{"fileEmbedGroup", "paragraph"}) {
		t.Fatalf("rich_content node types = %#v, want group plus trailing paragraph", types)
	}
	content := richContentPageContent(t, richContent[0])
	group, ok := content[0].(map[string]any)
	if !ok || group["type"] != "fileEmbedGroup" {
		t.Fatalf("first rich_content node = %#v, want fileEmbedGroup", content[0])
	}
	groupIDs := fileEmbedIDs([]map[string]any{{"description": group}})
	if !reflect.DeepEqual(groupIDs, []string{firstNewFileID, secondNewFileID}) {
		t.Fatalf("fileEmbedGroup ids = %#v, want new upload ids", groupIDs)
	}
}

func TestUpdate_FilePreservesAuthoredTrailingParagraph(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_old", Name: "Old Pack.zip"},
	}
	srv.existingRichContent = []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_old"}},
				map[string]any{
					"type": "paragraph",
					"content": []any{
						map[string]any{"type": "text", "text": "Important note"},
					},
				},
			},
		},
	}}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
		"--file-name", "New Pack.zip",
	})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	files := productUpdateJSONFiles(t, srv.putJSON)
	if len(files) != 2 {
		t.Fatalf("files payload len = %d, want 2", len(files))
	}
	newFileID := productUpdateNewUploadExternalID(t, files[1], "files[1]")
	if ids := richContentFileEmbedIDsFromBody(t, srv.putJSON); !reflect.DeepEqual(ids, []string{newFileID}) {
		t.Fatalf("rich_content fileEmbed ids = %#v, want new upload only", ids)
	}

	richContent := productUpdateJSONRichContent(t, srv.putJSON)
	if types := richContentNodeTypes(t, richContent[0]); !reflect.DeepEqual(types, []string{"fileEmbed", "paragraph"}) {
		t.Fatalf("rich_content node types = %#v, want file embed then authored paragraph", types)
	}
	content := richContentPageContent(t, richContent[0])
	paragraph := content[1].(map[string]any)
	textNodes, ok := paragraph["content"].([]any)
	if !ok || len(textNodes) != 1 {
		t.Fatalf("trailing paragraph content = %#v, want one text node", paragraph["content"])
	}
	textNode, ok := textNodes[0].(map[string]any)
	if !ok || textNode["text"] != "Important note" {
		t.Fatalf("trailing paragraph text node = %#v, want Important note", textNodes[0])
	}
}

func TestUpdate_FileAmbiguousEmbeddedReplacementErrorsBeforeUpload(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a", Name: "Old A.zip"},
		{ID: "file_b", Name: "Old B.zip"},
	}
	srv.existingRichContent = []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_a"}},
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_b"}},
			},
		},
	}}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "replacement bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous rich_content replacement error")
	}
	if !strings.Contains(err.Error(), "rich_content has 2 file embeds") || !strings.Contains(err.Error(), "pass one --file per existing file embed") {
		t.Fatalf("expected rich_content count error, got %v", err)
	}
	if srv.s3Calls.Load() != 0 {
		t.Fatalf("unexpected S3 calls: %d", srv.s3Calls.Load())
	}
	if srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected PUT calls: %d", srv.putCalls.Load())
	}
}

func TestUpdate_FileSurgeryFlagsAreRemoved(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	for _, flag := range []string{"--replace-files", "--remove-file", "--keep-file"} {
		cmd := newUpdateCmd()
		cmd.SetArgs([]string{"prod1", flag, "file_a"})
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected %s to be removed", flag)
		}
		if !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("expected unknown flag error for %s, got %v", flag, err)
		}
	}
	if srv.getCalls.Load() != 0 || srv.putCalls.Load() != 0 {
		t.Fatalf("unexpected API calls: get=%d put=%d", srv.getCalls.Load(), srv.putCalls.Load())
	}
}

func TestUpdate_FileDryRunPrefetchesButDoesNotUploadOrPut(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a", Name: "Old A.pdf"},
		{ID: "file_b", Name: "Old B.pdf"},
	}
	srv.existingRichContent = []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_a"}},
				map[string]any{"type": "paragraph"},
			},
		},
	}}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1", "--file", path})
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
	if len(payload.Preserved) != 2 || payload.Preserved[0].ID != "file_a" || payload.Preserved[1].ID != "file_b" {
		t.Fatalf("preserved = %+v", payload.Preserved)
	}
	if len(payload.Removed) != 0 {
		t.Fatalf("removed = %+v", payload.Removed)
	}
	files := productUpdateJSONFiles(t, payload.Request.Body)
	if len(files) != 3 || files[0]["id"] != "file_a" || files[1]["id"] != "file_b" {
		t.Fatalf("dry-run files payload = %#v", files)
	}
	if files[2]["url"] != "<uploaded:file:0>" {
		t.Fatalf("dry-run upload placeholder = %#v", files[2]["url"])
	}
	newFileID := productUpdateNewUploadExternalID(t, files[2], "files[2]")
	richContent := productUpdateJSONRichContent(t, payload.Request.Body)
	if ids := fileEmbedIDs(richContent); !reflect.DeepEqual(ids, []string{newFileID}) {
		t.Fatalf("dry-run fileEmbed ids = %#v, want new upload only", ids)
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

func TestUpdate_FileDryRunHumanIncludesExistingFileNames(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{
		{ID: "file_a", Name: "Old A.pdf"},
		{ID: "file_b", Name: "Old B.pdf"},
	}
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "fresh bytes")
	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"prod1", "--file", path})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, expected := range []string{
		"Preserve existing file: Old A.pdf (file_a)",
		"Preserve existing file: Old B.pdf (file_b)",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("human dry-run missing %q: %q", expected, out)
		}
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

func TestUpdate_InvalidUploadFailsBeforePrefetch(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	srv.existingFiles = []existingProductFile{{ID: "file_a"}}
	testutil.Setup(t, srv.dispatch(t))

	missingPath := filepath.Join(t.TempDir(), "missing.bin")
	cmd := testutil.Command(newUpdateCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"prod1", "--file", missingPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected local file validation error")
	}
	if !strings.Contains(err.Error(), "could not stat file") {
		t.Fatalf("expected stat error, got %v", err)
	}
	if srv.getCalls.Load() != 0 {
		t.Fatalf("unexpected GET calls: %d", srv.getCalls.Load())
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

func TestUpdate_RenderFailureDoesNotIncludeUploadedURLs(t *testing.T) {
	srv := newProductUpdateFileServers(t)
	testutil.Setup(t, srv.dispatch(t))

	path := writeProductUploadFixture(t, "first")
	cmd := testutil.Command(newUpdateCmd(), testutil.Yes(true), testutil.JSONOutput(), testutil.JQ(".bad[syntax"))
	cmd.SetArgs([]string{
		"prod1",
		"--file", path,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected render failure")
	}
	if !strings.Contains(err.Error(), "invalid jq expression") {
		t.Fatalf("expected jq error, got %v", err)
	}
	if strings.Contains(err.Error(), "https://example.com/attachments/u/k/original/upload-1.bin") {
		t.Fatalf("unexpected uploaded URL in render error: %v", err)
	}
	if srv.putCalls.Load() != 1 {
		t.Fatalf("PUT calls = %d, want 1", srv.putCalls.Load())
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

func TestRenderProductUpdateDryRun_JSONDirect(t *testing.T) {
	var buf bytes.Buffer
	opts := testutil.TestOptions(testutil.Stdout(&buf), testutil.JSONOutput())
	body := map[string]any{"files": []map[string]any{}}

	if err := renderProductUpdateDryRun(opts, "/products/prod1", productFileUpdatePlan{}, nil, body); err != nil {
		t.Fatalf("renderProductUpdateDryRun: %v", err)
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
