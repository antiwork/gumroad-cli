package products

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

type productMediaAttachError struct {
	cause                error
	productID            string
	completedAction      string
	partialMediaAttached bool
	retryCommands        []string
}

func (e *productMediaAttachError) Error() string {
	retry := strings.Join(e.retryCommands, "; ")
	if e.completedAction != "" {
		context := fmt.Sprintf("%s completed for product %s", e.completedAction, e.productID)
		if e.partialMediaAttached {
			context += "; some media was already attached"
		}
		return fmt.Sprintf("%v (%s; retry remaining media with: %s)", e.cause, context, retry)
	}
	if e.partialMediaAttached {
		return fmt.Sprintf("%v (some media was already attached to product %s; retry remaining media with: %s)", e.cause, e.productID, retry)
	}
	return e.cause.Error()
}

func (e *productMediaAttachError) Unwrap() error {
	return e.cause
}

func uploadAndAttachProductMedia(
	opts cmdutil.Options,
	client *api.Client,
	productID string,
	media []plannedProductMedia,
	completedAction string,
) ([]productMediaAttachmentResult, error) {
	results := make([]productMediaAttachmentResult, 0, len(media))
	for i, current := range media {
		signedID, err := directUploadProductMedia(opts, client, current)
		if err != nil {
			return results, wrapProductMediaAttachError(err, productID, completedAction, len(results) > 0, media[i:])
		}

		path := productMediaAttachPath(productID, current.Kind)
		params := url.Values{}
		params.Set("signed_blob_id", signedID)
		data, err := client.Post(path, params)
		if err != nil {
			return results, wrapProductMediaAttachError(err, productID, completedAction, len(results) > 0, media[i:])
		}
		results = append(results, productMediaAttachmentResult{
			Kind:     string(current.Kind),
			Path:     current.Path,
			Endpoint: path,
			Response: normalizeJSONForEmbedding(data),
		})
	}
	return results, nil
}

func wrapProductMediaAttachError(err error, productID, completedAction string, partialMediaAttached bool, media []plannedProductMedia) error {
	if completedAction == "" && !partialMediaAttached {
		return err
	}
	retryCommands := productMediaRetryCommands(productID, media)
	if len(retryCommands) == 0 {
		return err
	}
	return &productMediaAttachError{
		cause:                err,
		productID:            productID,
		completedAction:      completedAction,
		partialMediaAttached: partialMediaAttached,
		retryCommands:        retryCommands,
	}
}

func productMediaAttachPath(productID string, kind productMediaKind) string {
	switch kind {
	case productMediaThumbnail:
		return cmdutil.JoinPath("products", productID, "thumbnail")
	default:
		return cmdutil.JoinPath("products", productID, "covers")
	}
}

func productMediaRetryCommand(productID string, media plannedProductMedia) string {
	quotedPath := shellQuote(media.Path)
	switch media.Kind {
	case productMediaThumbnail:
		return fmt.Sprintf("gumroad products thumbnail set %s --image %s", productID, quotedPath)
	default:
		return fmt.Sprintf("gumroad products covers add %s --image %s", productID, quotedPath)
	}
}

func productMediaRetryCommands(productID string, media []plannedProductMedia) []string {
	commands := make([]string, 0, len(media))
	for _, current := range media {
		commands = append(commands, productMediaRetryCommand(productID, current))
	}
	return commands
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '@' || r == '+' || r == '=' ||
			(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func mergeProductMediaResult(data json.RawMessage, media []productMediaAttachmentResult) (json.RawMessage, error) {
	body := map[string]any{}
	normalized := normalizeJSONForEmbedding(data)
	if len(normalized) > 0 && string(normalized) != "null" {
		if err := json.Unmarshal(normalized, &body); err != nil {
			return nil, fmt.Errorf("could not parse response: %w", err)
		}
	}
	if len(media) > 0 {
		body["media"] = media
	}
	return json.Marshal(body)
}

func productMediaOnlyUpdateResult(productID string) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"success": true,
		"product": map[string]string{"id": productID},
	})
}

func normalizeJSONForEmbedding(data json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(data)) == 0 {
		return json.RawMessage("null")
	}
	return data
}
