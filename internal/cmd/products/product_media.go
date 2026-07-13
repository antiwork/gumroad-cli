package products

import (
	"encoding/json"
)

type productMediaKind string

const (
	productMediaCover     productMediaKind = "cover"
	productMediaPreview   productMediaKind = "preview"
	productMediaThumbnail productMediaKind = "thumbnail"

	maxDirectUploadErrorBody = 4 * 1024
)

type requestedProductMedia struct {
	Kind    productMediaKind
	Path    string
	IsVideo bool
}

type plannedProductMedia struct {
	requestedProductMedia
	Filename    string
	ContentType string
	Checksum    string
	Size        int64
}

type directUploadResponse struct {
	SignedID     string `json:"signed_id"`
	DirectUpload struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"direct_upload"`
}

type productMediaAttachmentResult struct {
	Kind     string          `json:"kind"`
	Path     string          `json:"path"`
	Endpoint string          `json:"endpoint"`
	Response json.RawMessage `json:"response"`
}
