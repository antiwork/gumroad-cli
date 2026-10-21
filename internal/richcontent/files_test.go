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

func TestAppendFileEmbedsToPageTargetsNamedPageOnly(t *testing.T) {
	richContent := []map[string]any{
		{
			"id":    "page_1",
			"title": "Start",
			"description": map[string]any{
				"type":    "doc",
				"content": []any{map[string]any{"type": "fileEmbed", "attrs": map[string]any{"id": "file_old", "uid": "old-uid"}}},
			},
		},
		{
			"id":    "page_2",
			"title": "Files",
			"description": map[string]any{
				"type":    "doc",
				"content": []any{map[string]any{"type": "paragraph"}},
			},
		},
		{
			"id":          "page_3",
			"title":       "Finish",
			"description": map[string]any{"type": "doc", "content": []any{}},
		},
	}

	next, err := AppendFileEmbedsToPage(richContent, "page_2", []FileRef{
		{FileID: "file_new_a", EmbedUID: "uid-a"},
		{FileID: "file_new_b", EmbedUID: "uid-b"},
	})
	if err != nil {
		t.Fatalf("AppendFileEmbedsToPage failed: %v", err)
	}

	if ids := FileEmbedIDs(next[:1]); !reflect.DeepEqual(ids, []string{"file_old"}) {
		t.Fatalf("page_1 file embed ids = %#v, want untouched", ids)
	}
	if ids := FileEmbedIDs(next[1:2]); !reflect.DeepEqual(ids, []string{"file_new_a", "file_new_b"}) {
		t.Fatalf("page_2 file embed ids = %#v, want both new files", ids)
	}
	if ids := FileEmbedIDs(next[2:]); len(ids) != 0 {
		t.Fatalf("page_3 file embed ids = %#v, want none", ids)
	}
	if ids := FileEmbedIDs(richContent[1:2]); len(ids) != 0 {
		t.Fatal("AppendFileEmbedsToPage mutated the input")
	}
}

func TestAppendFileEmbedsToPageRejectsUnknownPage(t *testing.T) {
	richContent := []map[string]any{{"id": "page_1", "description": map[string]any{"type": "doc"}}}

	if _, err := AppendFileEmbedsToPage(richContent, "missing", []FileRef{{FileID: "file_new", EmbedUID: "uid"}}); err == nil {
		t.Fatal("expected an error for an unknown page id")
	}
}

// A page a seller created but never filled in has no content array (or none at all),
// and must still accept files while keeping the rest of its fields.
func TestAddFileEmbedsToPageInitializesEmptyPages(t *testing.T) {
	refs := []FileRef{{FileID: "file_a", EmbedUID: "uid_a"}}
	cases := map[string]map[string]any{
		"doc without content": {"id": "page_1", "title": "Week 1", "description": map[string]any{"type": "doc"}},
		"null content":        {"id": "page_1", "title": "Week 1", "description": map[string]any{"type": "doc", "content": nil}},
		"no description":      {"id": "page_1", "title": "Week 1"},
	}
	for name, page := range cases {
		t.Run(name, func(t *testing.T) {
			AddFileEmbedsToPage(page, refs)

			if page["title"] != "Week 1" {
				t.Fatalf("title = %#v, want Week 1", page["title"])
			}
			description, ok := page["description"].(map[string]any)
			if !ok || description["type"] != "doc" {
				t.Fatalf("description = %#v, want a doc", page["description"])
			}
			content, ok := description["content"].([]any)
			if !ok || len(content) != 1 {
				t.Fatalf("content = %#v, want one embed", description["content"])
			}
			if ids := FileEmbedIDs([]map[string]any{page}); len(ids) != 1 || ids[0] != "file_a" {
				t.Fatalf("file embed ids = %#v, want [file_a]", ids)
			}
		})
	}
}

func TestAddFileEmbedsToPageKeepsExistingContent(t *testing.T) {
	page := map[string]any{"id": "page_1", "description": map[string]any{
		"type":    "doc",
		"content": []any{map[string]any{"type": "paragraph"}},
	}}

	AddFileEmbedsToPage(page, []FileRef{{FileID: "file_a", EmbedUID: "uid_a"}})

	content, _ := page["description"].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content nodes = %d, want 2", len(content))
	}
}
