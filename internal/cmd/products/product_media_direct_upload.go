package products

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

func directUploadProductMedia(opts cmdutil.Options, client *api.Client, media plannedProductMedia) (string, error) {
	params := url.Values{}
	params.Set("blob[filename]", media.Filename)
	params.Set("blob[byte_size]", strconv.FormatInt(media.Size, 10))
	params.Set("blob[checksum]", media.Checksum)
	params.Set("blob[content_type]", media.ContentType)

	data, err := client.Post("/direct_uploads", params)
	if err != nil {
		return "", err
	}
	resp, err := cmdutil.DecodeJSON[directUploadResponse](data)
	if err != nil {
		return "", err
	}
	if resp.SignedID == "" {
		return "", fmt.Errorf("direct upload response did not include signed_id")
	}
	if resp.DirectUpload.URL == "" {
		return "", fmt.Errorf("direct upload response did not include upload URL")
	}
	if err := putDirectUpload(opts, media, resp.DirectUpload.URL, resp.DirectUpload.Headers); err != nil {
		return "", err
	}
	return resp.SignedID, nil
}

func putDirectUpload(opts cmdutil.Options, media plannedProductMedia, uploadURL string, headers map[string]string) error {
	file, err := os.Open(media.Path)
	if err != nil {
		return fmt.Errorf("could not open %s image: %w", media.Kind, err)
	}
	defer func() { _ = file.Close() }()

	req, err := http.NewRequestWithContext(opts.Context, http.MethodPut, uploadURL, file)
	if err != nil {
		return fmt.Errorf("could not create direct upload request: %w", err)
	}
	req.ContentLength = media.Size
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", media.ContentType)
	}
	if req.Header.Get("Content-MD5") == "" {
		req.Header.Set("Content-MD5", media.Checksum)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("direct upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDirectUploadErrorBody))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("direct upload failed with HTTP %d: %s", resp.StatusCode, message)
	}
	return nil
}
