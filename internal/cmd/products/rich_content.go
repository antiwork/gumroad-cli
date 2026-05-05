package products

import (
	"crypto/rand"
	"fmt"
)

const defaultCreateRichContentTitle = "Page 1"

type createRichContentFileRef struct {
	FileID   string
	EmbedUID string
}

func newCreateRichContentFileRefs(count int) ([]createRichContentFileRef, error) {
	refs := make([]createRichContentFileRef, count)
	for i := range refs {
		fileUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file id: %w", err)
		}
		embedUUID, err := newUUIDV4()
		if err != nil {
			return nil, fmt.Errorf("could not generate file embed id: %w", err)
		}
		refs[i] = createRichContentFileRef{
			FileID:   "cli-upload-" + fileUUID,
			EmbedUID: embedUUID,
		}
	}
	return refs, nil
}

func buildCreateFileRichContent(fileRefs []createRichContentFileRef) []map[string]any {
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
		"title": defaultCreateRichContentTitle,
		"description": map[string]any{
			"type":    "doc",
			"content": content,
		},
	}}
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
