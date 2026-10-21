package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

// courseUploadServers is the smallest fake that behaves like the real product write:
// files are registered by external_id+url on each PUT and get a server id, so the next
// page's payload preserves them by id exactly as the real API requires.
type courseUploadServers struct {
	s3 *httptest.Server

	presignCalls atomic.Int32
	completeSeq  atomic.Int32
	getCalls     atomic.Int32
	putCalls     atomic.Int32
	putBodies    []map[string]any
	files        []existingProductFile
	richContent  []map[string]any
	failPutOn    int32
}

func newCourseUploadServers(t *testing.T) *courseUploadServers {
	t.Helper()

	s := &courseUploadServers{}
	s.s3 = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func (s *courseUploadServers) dispatch(t *testing.T) http.HandlerFunc {
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
						"files":                                  s.files,
						"rich_content":                           s.richContent,
						"has_same_rich_content_for_all_variants": true,
						"variants":                               []map[string]any{},
					},
				})
			case http.MethodPut:
				seq := s.putCalls.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode PUT body: %v", err)
				}
				if s.failPutOn != 0 && seq == s.failPutOn {
					http.Error(w, "boom", http.StatusBadRequest)
					return
				}
				s.putBodies = append(s.putBodies, body)
				s.registerFiles(t, body)
				if raw, ok := body["rich_content"].([]any); ok {
					content := make([]map[string]any, len(raw))
					for i, page := range raw {
						content[i], _ = page.(map[string]any)
					}
					s.richContent = rewriteEmbedIDs(content)
				}
				testutil.JSON(t, w, map[string]any{"success": true})
			default:
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
		case "/files/presign":
			s.presignCalls.Add(1)
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

func (s *courseUploadServers) registerFiles(t *testing.T, body map[string]any) {
	t.Helper()

	raw, ok := body["files"].([]any)
	if !ok {
		t.Fatalf("files payload has wrong type: %T", body["files"])
	}
	for _, current := range raw {
		file, _ := current.(map[string]any)
		if id, _ := file["id"].(string); id != "" {
			continue
		}
		externalID, _ := file["external_id"].(string)
		if externalID == "" {
			t.Fatalf("new file entry has no external_id: %#v", file)
		}
		s.files = append(s.files, existingProductFile{ID: "file-" + externalID, Name: "upload"})
	}
}

func writeCourseFiles(t *testing.T, dir string, count int) []string {
	t.Helper()

	paths := make([]string, count)
	for i := 0; i < count; i++ {
		path := filepath.Join(dir, fmt.Sprintf("lesson-%d.mp4", i+1))
		if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		paths[i] = path
	}
	return paths
}

func writeCourseManifest(t *testing.T, dir string, pages map[string][]string) string {
	t.Helper()

	var ordered []map[string]any
	for _, pageID := range []string{"week-1", "week-2", "week-3"} {
		files, ok := pages[pageID]
		if !ok {
			continue
		}
		entries := make([]any, len(files))
		for i, file := range files {
			entries[i] = file
		}
		ordered = append(ordered, map[string]any{"id": pageID, "title": strings.ToUpper(pageID), "files": entries})
	}
	data, err := json.Marshal(map[string]any{"pages": ordered})
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	path := filepath.Join(dir, "course.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func courseEmbedCounts(t *testing.T, richContent []map[string]any) map[string]int {
	t.Helper()

	counts := map[string]int{}
	for _, page := range richContent {
		pageID, _ := page["id"].(string)
		counts[pageID] = len(pageFileEmbedIDs(t, page))
	}
	return counts
}

func pageFileEmbedIDs(t *testing.T, page map[string]any) []string {
	t.Helper()

	var ids []string
	var walk func(node any)
	walk = func(node any) {
		current, ok := node.(map[string]any)
		if !ok {
			return
		}
		if attrs, ok := current["attrs"].(map[string]any); ok && current["type"] == "fileEmbed" {
			if id, _ := attrs["id"].(string); id != "" {
				ids = append(ids, id)
			}
		}
		if children, ok := current["content"].([]any); ok {
			for _, child := range children {
				walk(child)
			}
		}
	}
	walk(page["description"])
	return ids
}

func runCourseUploadCmd(t *testing.T, manifestPath string) error {
	t.Helper()

	cmd := testutil.Command(newContentUploadCmd())
	cmd.SetArgs([]string{"prod1", manifestPath})
	return cmd.Execute()
}

// The headline contract from gumroad-private#3835: a 3-page, 10-file course costs
// exactly 3 product writes, and every file lands on its own page once.
func TestContentUpload_OneProductWritePerPage(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 10)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{
		"week-1": files[0:3],
		"week-2": files[3:7],
		"week-3": files[7:10],
	})

	if err := runCourseUploadCmd(t, manifestPath); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got := s.putCalls.Load(); got != 3 {
		t.Fatalf("product writes = %d, want 3 (one per page, not one per file)", got)
	}
	if got := s.presignCalls.Load(); got != 10 {
		t.Fatalf("uploads = %d, want 10", got)
	}

	counts := courseEmbedCounts(t, s.richContent)
	if len(s.richContent) != 3 {
		t.Fatalf("rich content pages = %d, want 3: %#v", len(s.richContent), counts)
	}
	for page, want := range map[string]int{"week-1": 3, "week-2": 4, "week-3": 3} {
		if counts[page] != want {
			t.Fatalf("page %s embeds = %d, want %d (%#v)", page, counts[page], want, counts)
		}
	}

	// Each write preserves the files registered by the earlier pages.
	for i, wantPreserved := range []int{0, 3, 7} {
		payload, ok := s.putBodies[i]["files"].([]any)
		if !ok {
			t.Fatalf("write %d has no files payload", i+1)
		}
		preserved := 0
		for _, entry := range payload {
			if file, _ := entry.(map[string]any); file["id"] != nil {
				preserved++
			}
		}
		if preserved != wantPreserved {
			t.Fatalf("write %d preserved %d files, want %d", i+1, preserved, wantPreserved)
		}
	}

	// Every embed points at a registered file, with no duplicates.
	seen := map[string]bool{}
	for _, page := range s.richContent {
		for _, id := range pageFileEmbedIDs(t, page) {
			if seen[id] {
				t.Fatalf("file %s embedded twice", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != 10 {
		t.Fatalf("distinct embedded files = %d, want 10", len(seen))
	}
	assertEmbedsResolve(t, s)
}

// A rerun after an interruption must not upload or place anything twice: the second
// run reuses the recorded upload URLs and skips the pages already written.
func TestContentUpload_ResumeAfterInterruption(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 10)
	s := newCourseUploadServers(t)
	s.failPutOn = 2
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{
		"week-1": files[0:3],
		"week-2": files[3:7],
		"week-3": files[7:10],
	})

	if err := runCourseUploadCmd(t, manifestPath); err == nil {
		t.Fatal("expected the interrupted run to fail on the second page write")
	}
	if got := s.putCalls.Load(); got != 2 {
		t.Fatalf("first run writes = %d, want 2 (page 1 ok, page 2 failed)", got)
	}
	if got := s.presignCalls.Load(); got != 7 {
		t.Fatalf("first run uploads = %d, want 7 (page 1 + page 2 files)", got)
	}

	s.failPutOn = 0
	if err := runCourseUploadCmd(t, manifestPath); err != nil {
		t.Fatalf("resumed run failed: %v", err)
	}

	if got := s.presignCalls.Load(); got != 10 {
		t.Fatalf("uploads across both runs = %d, want 10 (page 3 files only on rerun)", got)
	}
	if got := s.putCalls.Load(); got != 4 {
		t.Fatalf("writes across both runs = %d, want 4 (1 + failed + 2)", got)
	}

	counts := courseEmbedCounts(t, s.richContent)
	for page, want := range map[string]int{"week-1": 3, "week-2": 4, "week-3": 3} {
		if counts[page] != want {
			t.Fatalf("page %s embeds = %d, want %d after resume (%#v)", page, counts[page], want, counts)
		}
	}
}

// A manifest that names a file we cannot read must fail before the first upload, the
// same guard `--page` has for an unknown page id.
func TestContentUpload_RejectsMissingFileBeforeUpload(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 2)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{
		"week-1": {files[0], filepath.Join(dir, "missing.mp4")},
	})

	err := runCourseUploadCmd(t, manifestPath)
	if err == nil || !strings.Contains(err.Error(), "missing.mp4") {
		t.Fatalf("expected a missing-file error, got %v", err)
	}
	if got := s.presignCalls.Load(); got != 0 {
		t.Fatalf("uploads = %d, want 0 (validation must run before any upload)", got)
	}
	if got := s.putCalls.Load(); got != 0 {
		t.Fatalf("product writes = %d, want 0", got)
	}
}

func TestContentUpload_DryRunPlansPageWrites(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 4)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{
		"week-1": files[0:2],
		"week-2": files[2:4],
	})

	// Dry run goes through the same command with the global --dry-run flag set.
	dryCmd := testutil.Command(newContentUploadCmd(), testutil.JSONOutput(), testutil.DryRun(true))
	dryCmd.SetArgs([]string{"prod1", manifestPath})
	out := testutil.CaptureStdout(func() {
		if err := dryCmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	var summary courseUploadSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, out)
	}
	if summary.Pages != 2 || summary.Files != 4 || !summary.DryRun {
		t.Fatalf("unexpected dry-run summary: %#v", summary)
	}
	if got := s.presignCalls.Load(); got != 0 {
		t.Fatalf("dry run uploaded %d files, want 0", got)
	}
	if got := s.putCalls.Load(); got != 0 {
		t.Fatalf("dry run wrote the product %d times, want 0", got)
	}
}

func TestParseCourseManifest_RejectsBadManifests(t *testing.T) {
	cases := map[string]string{
		"invalid JSON":              `{`,
		"no pages":                  `{"pages":[]}`,
		"page without id":           `{"pages":[{"title":"Week 1","files":[]}]}`,
		"file without path":         `{"pages":[{"id":"w1","files":[{"name":"Notes"}]}]}`,
		"empty file path":           `{"pages":[{"id":"w1","files":[""]}]}`,
		"file not a path or object": `{"pages":[{"id":"w1","files":[42]}]}`,
	}
	for name, body := range cases {
		if _, err := parseCourseManifest([]byte(body)); err == nil {
			t.Fatalf("%s: expected an error, got none", name)
		}
	}
}

func TestParseCourseManifest_AcceptsPathAndObjectFiles(t *testing.T) {
	manifest, err := parseCourseManifest([]byte(
		`{"pages":[{"page":"w1","files":["a.mp4",{"path":"b.pdf","name":"Notes","description":"Week brief"}]}]}`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(manifest.Pages) != 1 || manifest.Pages[0].ID != "w1" {
		t.Fatalf("unexpected pages: %#v", manifest.Pages)
	}
	files := manifest.Pages[0].Files
	if len(files) != 2 || files[0].Path != "a.mp4" || files[1].DisplayName != "Notes" || files[1].Description != "Week brief" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestContentUpload_RejectsDuplicatePagesAndDirectories(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 1)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	duplicate := filepath.Join(dir, "duplicate.json")
	if err := os.WriteFile(duplicate, []byte(fmt.Sprintf(
		`{"pages":[{"id":"w1","files":[%q]},{"id":"w1","files":[%q]}]}`, files[0], files[0])), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := runCourseUploadCmd(t, duplicate); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a duplicate-page error, got %v", err)
	}

	directory := filepath.Join(dir, "directory.json")
	if err := os.WriteFile(directory, []byte(fmt.Sprintf(
		`{"pages":[{"id":"w1","files":[%q]}]}`, dir)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := runCourseUploadCmd(t, directory); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected a directory error, got %v", err)
	}
	if got := s.presignCalls.Load(); got != 0 {
		t.Fatalf("uploads = %d, want 0", got)
	}
}

func TestContentUpload_RejectsPerVariantProduct(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 1)
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"id":                                     "prod1",
				"has_same_rich_content_for_all_variants": false,
				"variants":                               []map[string]any{{"options": []map[string]any{{"name": "Blue"}}}},
			},
		})
	})

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files})
	err := runCourseUploadCmd(t, manifestPath)
	if err == nil || !strings.Contains(err.Error(), "per-variant") {
		t.Fatalf("expected a per-variant guard error, got %v", err)
	}
}

func TestContentUpload_StateProductMismatch(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 1)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files})
	statePath := manifestPath + defaultCourseStateSuffix
	if err := writeCourseUploadState(statePath, courseUploadState{Version: 1, ProductID: "other"}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err := runCourseUploadCmd(t, manifestPath)
	if err == nil || !strings.Contains(err.Error(), "belongs to product other") {
		t.Fatalf("expected a resume-file product mismatch, got %v", err)
	}
}

func TestReadCourseUploadState_RejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if _, err := readCourseUploadState(path); err == nil {
		t.Fatal("expected a corrupt resume file to fail")
	}
}

func TestContentUpload_TextOutput(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 2)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files})

	cmd := testutil.Command(newContentUploadCmd())
	cmd.SetArgs([]string{"prod1", manifestPath})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})
	if !strings.Contains(out, "Uploaded 2 files and wrote 1 pages") {
		t.Fatalf("unexpected summary: %s", out)
	}

	dryDir := t.TempDir()
	dryFiles := writeCourseFiles(t, dryDir, 1)
	dryManifest := writeCourseManifest(t, dryDir, map[string][]string{"week-1": dryFiles})
	dryCmd := testutil.Command(newContentUploadCmd(), testutil.DryRun(true))
	dryCmd.SetArgs([]string{"prod1", dryManifest})
	dryOut := testutil.CaptureStdout(func() {
		if err := dryCmd.Execute(); err != nil {
			t.Fatalf("dry run failed: %v", err)
		}
	})
	if !strings.Contains(dryOut, "1 product writes") || !strings.Contains(dryOut, "No files uploaded.") {
		t.Fatalf("unexpected dry-run output: %s", dryOut)
	}
}

func rewriteEmbedIDs(pages []map[string]any) []map[string]any {
	for _, page := range pages {
		description, _ := page["description"].(map[string]any)
		nodes, _ := description["content"].([]any)
		for _, node := range nodes {
			nodeMap, _ := node.(map[string]any)
			attrs, _ := nodeMap["attrs"].(map[string]any)
			if id, _ := attrs["id"].(string); strings.HasPrefix(id, "cli-upload-") {
				attrs["id"] = "file-" + id
			}
		}
	}
	return pages
}

func assertEmbedsResolve(t *testing.T, s *courseUploadServers) {
	t.Helper()
	registered := map[string]bool{}
	for _, file := range s.files {
		registered[file.ID] = true
	}
	for _, page := range s.richContent {
		for _, id := range pageFileEmbedIDs(t, page) {
			if !registered[id] {
				t.Fatalf("embed %s does not point at a registered file", id)
			}
		}
	}
}

func TestContentUpload_AppendsToExistingPageKeepingText(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 2)
	s := newCourseUploadServers(t)
	s.richContent = []map[string]any{{"id": "week-1", "title": "Old title", "description": map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Lesson notes"}}},
	}}}}
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files})
	if err := runCourseUploadCmd(t, manifestPath); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(s.richContent) != 1 || s.richContent[0]["title"] != "Old title" {
		t.Fatalf("existing page must keep its title: %#v", s.richContent)
	}
	raw, _ := json.Marshal(s.richContent[0])
	if !strings.Contains(string(raw), "Lesson notes") {
		t.Fatalf("existing lesson text lost: %s", raw)
	}
	if counts := courseEmbedCounts(t, s.richContent); counts["week-1"] != 2 {
		t.Fatalf("embeds = %d, want 2", counts["week-1"])
	}
	assertEmbedsResolve(t, s)
}

func TestContentUpload_ReuploadsReplacedFileOnResume(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 2)
	s := newCourseUploadServers(t)
	s.failPutOn = 1
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files})
	if err := runCourseUploadCmd(t, manifestPath); err == nil {
		t.Fatal("expected the first write to fail")
	}
	if err := os.WriteFile(files[0], []byte("replaced with longer bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.failPutOn = 0
	if err := runCourseUploadCmd(t, manifestPath); err != nil {
		t.Fatalf("resumed run failed: %v", err)
	}
	if got := s.presignCalls.Load(); got != 3 {
		t.Fatalf("uploads = %d, want 3 (2 first run + the replaced file)", got)
	}
}

func TestContentUpload_RejectsChangedWrittenPage(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 2)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files[:1]})
	if err := runCourseUploadCmd(t, manifestPath); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	manifestPath = writeCourseManifest(t, dir, map[string][]string{"week-1": files})
	err := runCourseUploadCmd(t, manifestPath)
	if err == nil || !strings.Contains(err.Error(), "changed after it was written") {
		t.Fatalf("expected a changed-page error, got %v", err)
	}
}

func TestContentUpload_RejectsEmptyFileBeforeUpload(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 1)
	empty := filepath.Join(dir, "empty.mp4")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": {files[0], empty}})
	err := runCourseUploadCmd(t, manifestPath)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected an empty-file error, got %v", err)
	}
	if got := s.presignCalls.Load(); got != 0 {
		t.Fatalf("uploads = %d, want 0", got)
	}
}

func TestContentUpload_DryRunSkipsWrittenPages(t *testing.T) {
	dir := t.TempDir()
	files := writeCourseFiles(t, dir, 2)
	s := newCourseUploadServers(t)
	testutil.Setup(t, s.dispatch(t))

	manifestPath := writeCourseManifest(t, dir, map[string][]string{"week-1": files[:1], "week-2": files[1:]})
	if err := runCourseUploadCmd(t, manifestPath); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	dryCmd := testutil.Command(newContentUploadCmd(), testutil.JSONOutput(), testutil.DryRun(true))
	dryCmd.SetArgs([]string{"prod1", manifestPath})
	out := testutil.CaptureStdout(func() {
		if err := dryCmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})
	var summary courseUploadSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.PagesToWrite != 0 || summary.PagesSkipped != 2 {
		t.Fatalf("unexpected dry-run summary: %#v", summary)
	}
}
