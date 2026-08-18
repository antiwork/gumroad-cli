package variants

import (
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/richcontent"
)

func TestCompleteFileIDMappingsRequiresEveryRef(t *testing.T) {
	refs := []richcontent.FileRef{{FileID: "cli-upload-1"}, {FileID: "cli-upload-2"}}

	_, err := completeFileIDMappings(refs, map[string]string{"cli-upload-1": "file_a"})
	if err == nil {
		t.Fatal("expected missing mapping error")
	}
	if !strings.Contains(err.Error(), "cli-upload-2") {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := completeFileIDMappings(refs, map[string]string{"cli-upload-1": "file_a", "cli-upload-2": "file_b", "extra": "ignored"})
	if err != nil {
		t.Fatalf("completeFileIDMappings failed: %v", err)
	}
	if got["cli-upload-1"] != "file_a" || got["cli-upload-2"] != "file_b" {
		t.Fatalf("complete mappings = %#v", got)
	}
}
