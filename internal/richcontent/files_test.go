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

func TestReplaceFileEmbedIDsRewritesMatchingEmbedsOnly(t *testing.T) {
	richContent := []map[string]any{{
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "cli-upload-1", "uid": "keep-uid"}},
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_keep"}},
			},
		},
	}}

	next, err := ReplaceFileEmbedIDs(richContent, map[string]string{"cli-upload-1": "file_real"})
	if err != nil {
		t.Fatalf("ReplaceFileEmbedIDs failed: %v", err)
	}
	if ids := FileEmbedIDs(next); !reflect.DeepEqual(ids, []string{"file_real", "file_keep"}) {
		t.Fatalf("file embed ids = %#v", ids)
	}
	attrs := next[0]["description"].(map[string]any)["content"].([]any)[0].(map[string]any)["attrs"].(map[string]any)
	if attrs["uid"] != "keep-uid" {
		t.Fatalf("uid = %#v, want keep-uid", attrs["uid"])
	}
	if FileEmbedIDs(richContent)[0] != "cli-upload-1" {
		t.Fatal("ReplaceFileEmbedIDs mutated the input")
	}
}
