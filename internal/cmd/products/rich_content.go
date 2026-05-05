package products

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

const defaultFileRichContentTitle = "Page 1"

type richContentFileRef struct {
	FileID   string
	EmbedUID string
}

func newRichContentFileRefs(count int) ([]richContentFileRef, error) {
	refs := make([]richContentFileRef, count)
	for i := range refs {
		fileUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file id: %w", err)
		}
		embedUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file embed id: %w", err)
		}
		refs[i] = richContentFileRef{
			FileID:   "cli-upload-" + fileUUID,
			EmbedUID: embedUUID,
		}
	}
	return refs, nil
}

func buildFileRichContent(fileRefs []richContentFileRef) []map[string]any {
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
		"title": defaultFileRichContentTitle,
		"description": map[string]any{
			"type":    "doc",
			"content": content,
		},
	}}
}

func buildProductUpdateRichContent(
	cmd *cobra.Command,
	existingRichContent []map[string]any,
	filePlan productFileUpdatePlan,
	fileRefs []richContentFileRef,
) ([]map[string]any, bool, error) {
	removedEmbeddedIDs := removedFileEmbedIDs(existingRichContent, filePlan.Removed)
	if len(removedEmbeddedIDs) > 0 {
		if len(fileRefs) == 0 {
			richContent, err := cloneRichContent(existingRichContent)
			if err != nil {
				return nil, false, err
			}
			stripFileEmbedIDs(richContent, removedEmbeddedIDs)
			return richContent, true, nil
		}
		if len(removedEmbeddedIDs) != 1 || len(fileRefs) != 1 {
			return nil, false, cmdutil.UsageErrorf(cmd,
				"cannot automatically update rich_content for embedded file removal (%s); replace one embedded file at a time with exactly one --remove-file and one --file",
				joinComma(removedEmbeddedIDs))
		}

		richContent, err := cloneRichContent(existingRichContent)
		if err != nil {
			return nil, false, err
		}
		replaceFileEmbedID(richContent, removedEmbeddedIDs[0], fileRefs[0].FileID)
		return richContent, true, nil
	}

	if len(fileRefs) == 0 {
		return nil, false, nil
	}

	richContent, err := appendFileEmbeds(existingRichContent, filePlan.Preserved, fileRefs)
	if err != nil {
		return nil, false, err
	}
	return richContent, true, nil
}

func removedFileEmbedIDs(richContent []map[string]any, removed []existingProductFile) []string {
	if len(richContent) == 0 || len(removed) == 0 {
		return nil
	}

	removedIDs := make(map[string]struct{}, len(removed))
	for _, file := range removed {
		removedIDs[file.ID] = struct{}{}
	}

	seen := map[string]struct{}{}
	for _, id := range fileEmbedIDs(richContent) {
		if _, ok := removedIDs[id]; !ok {
			continue
		}
		seen[id] = struct{}{}
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func appendFileEmbeds(richContent []map[string]any, preserved []existingProductFile, fileRefs []richContentFileRef) ([]map[string]any, error) {
	if len(fileRefs) == 0 {
		return cloneRichContent(richContent)
	}
	if len(richContent) == 0 {
		preservedRefs, err := richContentRefsForExistingFiles(preserved)
		if err != nil {
			return nil, err
		}
		return buildFileRichContent(append(preservedRefs, fileRefs...)), nil
	}

	cloned, err := cloneRichContent(richContent)
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

func richContentRefsForExistingFiles(files []existingProductFile) ([]richContentFileRef, error) {
	refs := make([]richContentFileRef, len(files))
	for i, file := range files {
		embedUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file embed id: %w", err)
		}
		refs[i] = richContentFileRef{
			FileID:   file.ID,
			EmbedUID: embedUUID,
		}
	}
	return refs, nil
}

func appendFileEmbedPage(richContent []map[string]any) int {
	target := len(richContent) - 1
	for i, page := range richContent {
		if richContentPageHasFileEmbed(page) {
			target = i
		}
	}
	return target
}

func richContentPageHasFileEmbed(page map[string]any) bool {
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

func appendFileEmbedsToContent(content []any, fileRefs []richContentFileRef) []any {
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

func cloneRichContent(richContent []map[string]any) ([]map[string]any, error) {
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

func fileEmbedNodes(fileRefs []richContentFileRef) []any {
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

func fileEmbedIDs(richContent []map[string]any) []string {
	var ids []string
	for _, page := range richContent {
		collectFileEmbedIDs(page["description"], &ids)
	}
	return ids
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

func replaceFileEmbedID(richContent []map[string]any, oldID, newID string) {
	for _, page := range richContent {
		replaceFileEmbedIDInNode(page["description"], oldID, newID)
	}
}

func stripFileEmbedIDs(richContent []map[string]any, ids []string) {
	if len(ids) == 0 {
		return
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, page := range richContent {
		stripFileEmbedIDsInNode(page["description"], idSet)
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

func nodeHasType(node any, typ string) bool {
	nodeMap, ok := node.(map[string]any)
	return ok && nodeMap["type"] == typ
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

func newUUIDV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
