package variants

import (
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/richcontent"
)

func TestNewFileIDsSinceKeepsAfterOrder(t *testing.T) {
	before := []variantExistingProductFile{{ID: "file_existing"}}
	after := []variantExistingProductFile{
		{ID: "file_existing"},
		{ID: "file_new_a"},
		{ID: "file_new_b"},
	}
	got := newFileIDsSince(before, after)
	if len(got) != 2 || got[0] != "file_new_a" || got[1] != "file_new_b" {
		t.Fatalf("newFileIDsSince = %#v, want [file_new_a file_new_b]", got)
	}
}

func TestFileIDReplacementsRequiresMatchingCount(t *testing.T) {
	refs := []richcontent.FileRef{{FileID: "cli-upload-1"}, {FileID: "cli-upload-2"}}
	_, err := fileIDReplacements(refs, []string{"file_only"})
	if err == nil {
		t.Fatal("expected count mismatch")
	}
	if !strings.Contains(err.Error(), "expected 2 new product file id") {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := fileIDReplacements(refs, []string{"file_a", "file_b"})
	if err != nil {
		t.Fatalf("fileIDReplacements failed: %v", err)
	}
	if got["cli-upload-1"] != "file_a" || got["cli-upload-2"] != "file_b" {
		t.Fatalf("replacements = %#v", got)
	}
}

func TestCompleteFileIDMappingsRequiresEveryRef(t *testing.T) {
	refs := []richcontent.FileRef{{FileID: "cli-upload-1"}, {FileID: "cli-upload-2"}}
	if got := completeFileIDMappings(refs, map[string]string{"cli-upload-1": "file_a"}); got != nil {
		t.Fatalf("partial mappings = %#v, want nil", got)
	}
	got := completeFileIDMappings(refs, map[string]string{"cli-upload-1": "file_a", "cli-upload-2": "file_b", "extra": "ignored"})
	if got["cli-upload-1"] != "file_a" || got["cli-upload-2"] != "file_b" {
		t.Fatalf("complete mappings = %#v", got)
	}
}
