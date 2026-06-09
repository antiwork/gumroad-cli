package richcontent

import (
	"reflect"
	"strings"
	"testing"
)

func TestRollFileEmbedsReplacesOnlyCountedEmbeds(t *testing.T) {
	richContent := []map[string]any{{
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_old", "uid": "old-uid", "collapsed": true, "label": "Keep me"}},
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"uid": "missing-id"}},
				map[string]any{"type": "paragraph"},
			},
		},
	}}

	next, err := RollFileEmbeds(richContent, nil, []FileRef{{FileID: "file_new", EmbedUID: "new-uid"}})
	if err != nil {
		t.Fatalf("RollFileEmbeds failed: %v", err)
	}
	if ids := FileEmbedIDs(next); !reflect.DeepEqual(ids, []string{"file_new"}) {
		t.Fatalf("file embed ids = %#v, want new counted embed only", ids)
	}

	content := next[0]["description"].(map[string]any)["content"].([]any)
	replaced := content[0].(map[string]any)
	if attrs := replaced["attrs"].(map[string]any); attrs["id"] != "file_new" || attrs["uid"] != "new-uid" || attrs["collapsed"] != true || attrs["label"] != "Keep me" {
		t.Fatalf("replaced attrs = %#v", attrs)
	}
	malformed := content[1].(map[string]any)
	if attrs := malformed["attrs"].(map[string]any); attrs["id"] != nil || attrs["uid"] != "missing-id" {
		t.Fatalf("malformed embed should remain untouched, got %#v", attrs)
	}
}

func TestRollFileEmbedsErrorsOnReplacementCountMismatch(t *testing.T) {
	richContent := []map[string]any{{
		"description": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_a"}},
				map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_b"}},
			},
		},
	}}

	_, err := RollFileEmbeds(richContent, nil, []FileRef{{FileID: "file_new", EmbedUID: "new-uid"}})
	if err == nil {
		t.Fatal("expected replacement count mismatch")
	}
	if !strings.Contains(err.Error(), "rich_content has 2 file embeds; got 1 replacement files") {
		t.Fatalf("unexpected error: %v", err)
	}
}
