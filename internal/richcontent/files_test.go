package richcontent

import (
	"reflect"
	"testing"
)

func TestAppendFileEmbedsKeepsExistingAndAddsNew(t *testing.T) {
	richContent := []map[string]any{{
		"id":    "page_1",
		"title": "Existing page",
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_old", "uid": "old-uid"}},
				map[string]any{"type": "paragraph"},
			},
		},
	}}

	next, err := AppendFileEmbeds(richContent, []string{"file_old"}, []FileRef{{FileID: "file_new", EmbedUID: "new-uid"}})
	if err != nil {
		t.Fatalf("AppendFileEmbeds failed: %v", err)
	}
	if ids := FileEmbedIDs(next); !reflect.DeepEqual(ids, []string{"file_old", "file_new"}) {
		t.Fatalf("file embed ids = %#v, want existing plus new", ids)
	}
	if next[0]["id"] != "page_1" {
		t.Fatalf("page id = %#v, want page_1", next[0]["id"])
	}
}
