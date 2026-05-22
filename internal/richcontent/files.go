package richcontent

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
)

const DefaultFileTitle = "Page 1"

type FileRef struct {
	FileID   string
	EmbedUID string
}

func NewFileRefs(count int) ([]FileRef, error) {
	refs := make([]FileRef, count)
	for i := range refs {
		fileUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file id: %w", err)
		}
		embedUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file embed id: %w", err)
		}
		refs[i] = FileRef{
			FileID:   "cli-upload-" + fileUUID,
			EmbedUID: embedUUID,
		}
	}
	return refs, nil
}

func BuildFileRichContent(fileRefs []FileRef) []map[string]any {
	content := make([]map[string]any, 0, len(fileRefs)+1)
	for _, ref := range fileRefs {
		content = append(content, map[string]any{
			"type": "fileEmbed",
			"attrs": map[string]any{
				"id":        ref.FileID,
				"uid":       ref.EmbedUID,
				"collapsed": false,
			},
		})
	}
	content = append(content, map[string]any{"type": "paragraph"})

	return []map[string]any{{
		"title": DefaultFileTitle,
		"description": map[string]any{
			"type":    "doc",
			"content": content,
		},
	}}
}

func AppendFileEmbeds(richContent []map[string]any, preservedFileIDs []string, fileRefs []FileRef) ([]map[string]any, error) {
	if len(fileRefs) == 0 {
		return Clone(richContent)
	}
	if len(richContent) == 0 {
		preservedRefs, err := refsForExistingFiles(preservedFileIDs)
		if err != nil {
			return nil, err
		}
		return BuildFileRichContent(append(preservedRefs, fileRefs...)), nil
	}

	cloned, err := Clone(richContent)
	if err != nil {
		return nil, err
	}

	page := cloned[appendFileEmbedPage(cloned)]
	description, ok := page["description"].(map[string]any)
	if !ok {
		description = map[string]any{"type": "doc"}
		page["description"] = description
	}
	content, _ := description["content"].([]any)
	if group := fileEmbedGroupForAppend(content); group != nil {
		groupContent, _ := group["content"].([]any)
		group["content"] = append(groupContent, fileEmbedNodes(fileRefs)...)
	} else {
		content = appendFileEmbedsToContent(content, fileRefs)
	}
	description["type"] = "doc"
	description["content"] = content
	return cloned, nil
}

func Clone(richContent []map[string]any) ([]map[string]any, error) {
	data, err := json.Marshal(richContent)
	if err != nil {
		return nil, fmt.Errorf("could not encode rich_content: %w", err)
	}
	var cloned []map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("could not decode rich_content: %w", err)
	}
	return cloned, nil
}

func FileEmbedIDs(richContent []map[string]any) []string {
	var ids []string
	for _, page := range richContent {
		collectFileEmbedIDs(page["description"], &ids)
	}
	return ids
}

func ReplaceFileEmbedID(richContent []map[string]any, oldID, newID string) {
	for _, page := range richContent {
		replaceFileEmbedIDInNode(page["description"], oldID, newID)
	}
}

func StripFileEmbedIDs(richContent []map[string]any, ids []string) {
	if len(ids) == 0 {
		return
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, page := range richContent {
		if description, ok := page["description"].(map[string]any); ok {
			stripFileEmbedIDsInNode(description, idSet)
			ensureDescriptionContent(description)
		}
	}
}

func refsForExistingFiles(fileIDs []string) ([]FileRef, error) {
	refs := make([]FileRef, len(fileIDs))
	for i, fileID := range fileIDs {
		embedUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file embed id: %w", err)
		}
		refs[i] = FileRef{
			FileID:   fileID,
			EmbedUID: embedUUID,
		}
	}
	return refs, nil
}

func appendFileEmbedPage(richContent []map[string]any) int {
	target := len(richContent) - 1
	for i, page := range richContent {
		if pageHasFileEmbed(page) {
			target = i
		}
	}
	return target
}

func pageHasFileEmbed(page map[string]any) bool {
	var ids []string
	collectFileEmbedIDs(page["description"], &ids)
	return len(ids) > 0
}

func fileEmbedGroupForAppend(content []any) map[string]any {
	var target map[string]any
	for _, child := range content {
		childMap, ok := child.(map[string]any)
		if !ok {
			continue
		}
		if childMap["type"] == "fileEmbed" {
			return nil
		}

		var ids []string
		collectFileEmbedIDs(childMap, &ids)
		if len(ids) == 0 {
			continue
		}
		if childMap["type"] != "fileEmbedGroup" || target != nil {
			return nil
		}
		target = childMap
	}
	return target
}

func appendFileEmbedsToContent(content []any, fileRefs []FileRef) []any {
	var trailingParagraph any
	if len(content) > 0 && nodeHasType(content[len(content)-1], "paragraph") {
		trailingParagraph = content[len(content)-1]
		content = content[:len(content)-1]
	}
	content = append(content, fileEmbedNodes(fileRefs)...)
	if trailingParagraph == nil {
		trailingParagraph = map[string]any{"type": "paragraph"}
	}
	return append(content, trailingParagraph)
}

func fileEmbedNodes(fileRefs []FileRef) []any {
	nodes := make([]any, len(fileRefs))
	for i, ref := range fileRefs {
		nodes[i] = map[string]any{
			"type": "fileEmbed",
			"attrs": map[string]any{
				"id":        ref.FileID,
				"uid":       ref.EmbedUID,
				"collapsed": false,
			},
		}
	}
	return nodes
}

func collectFileEmbedIDs(node any, ids *[]string) {
	current, ok := node.(map[string]any)
	if !ok {
		return
	}
	if current["type"] == "fileEmbed" {
		if attrs, ok := current["attrs"].(map[string]any); ok {
			if id, ok := attrs["id"].(string); ok && id != "" {
				*ids = append(*ids, id)
			}
		}
	}
	if children, ok := current["content"].([]any); ok {
		for _, child := range children {
			collectFileEmbedIDs(child, ids)
		}
	}
}

func replaceFileEmbedIDInNode(node any, oldID, newID string) {
	current, ok := node.(map[string]any)
	if !ok {
		return
	}
	if current["type"] == "fileEmbed" {
		if attrs, ok := current["attrs"].(map[string]any); ok && attrs["id"] == oldID {
			attrs["id"] = newID
		}
	}
	if children, ok := current["content"].([]any); ok {
		for _, child := range children {
			replaceFileEmbedIDInNode(child, oldID, newID)
		}
	}
}

func stripFileEmbedIDsInNode(node any, ids map[string]struct{}) {
	current, ok := node.(map[string]any)
	if !ok {
		return
	}
	children, ok := current["content"].([]any)
	if !ok {
		return
	}

	kept := make([]any, 0, len(children))
	for _, child := range children {
		if childMap, ok := child.(map[string]any); ok {
			if childMap["type"] == "fileEmbed" {
				if attrs, ok := childMap["attrs"].(map[string]any); ok {
					if id, ok := attrs["id"].(string); ok {
						if _, remove := ids[id]; remove {
							continue
						}
					}
				}
			}
			stripFileEmbedIDsInNode(childMap, ids)
			if isEmptyFileEmbedGroup(childMap) {
				continue
			}
		}
		kept = append(kept, child)
	}
	current["content"] = kept
}

func isEmptyFileEmbedGroup(node map[string]any) bool {
	if node["type"] != "fileEmbedGroup" {
		return false
	}
	children, ok := node["content"].([]any)
	return !ok || len(children) == 0
}

func ensureDescriptionContent(description map[string]any) {
	content, ok := description["content"].([]any)
	if ok && len(content) > 0 {
		return
	}
	description["content"] = []any{map[string]any{"type": "paragraph"}}
}

func nodeHasType(node any, typ string) bool {
	nodeMap, ok := node.(map[string]any)
	return ok && nodeMap["type"] == typ
}

func newUUIDV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
