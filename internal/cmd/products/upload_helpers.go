package products

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
)

// s3HTTPClientForTesting redirects multipart PUTs at a test TLS server. Tests
// in this package must not use t.Parallel while mutating this hook.
var s3HTTPClientForTesting *http.Client

type batchUploadInput struct {
	Path string
	Plan upload.Plan
}

type partialUploadError struct {
	cause        error
	uploadedURLs []string
}

func (e *partialUploadError) Error() string {
	if len(e.uploadedURLs) == 0 {
		return e.cause.Error()
	}
	return fmt.Sprintf("%v (previously uploaded file URLs: %s)", e.cause, strings.Join(e.uploadedURLs, ", "))
}

func (e *partialUploadError) Unwrap() error {
	return e.cause
}

func uploadBatch(opts cmdutil.Options, client *api.Client, uploads []batchUploadInput) ([]string, error) {
	urls := make([]string, len(uploads))
	for i, current := range uploads {
		statusLabel := current.Plan.Filename
		if len(uploads) > 1 {
			statusLabel = fmt.Sprintf("%s (%d/%d)", current.Plan.Filename, i+1, len(uploads))
		}

		fileURL, err := uploadui.UploadFile(opts, client, current.Path, current.Plan, s3HTTPClientForTesting, statusLabel)
		if err != nil {
			return nil, wrapPartialUploadError(err, urls[:i])
		}
		urls[i] = fileURL
	}
	return urls, nil
}

func wrapPartialUploadError(err error, uploadedURLs []string) error {
	if err == nil {
		return nil
	}
	if len(uploadedURLs) == 0 {
		return err
	}
	copied := append([]string(nil), uploadedURLs...)
	return &partialUploadError{
		cause:        err,
		uploadedURLs: copied,
	}
}

func buildProductJSONBody(params url.Values, files []map[string]any) map[string]any {
	body := make(map[string]any, len(params)+1)
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if additionalDetailParamPattern.MatchString(key) {
			// Gathered into rows below, so the body carries the objects, not the form keys.
			continue
		}
		values := append([]string(nil), params[key]...)
		switch key {
		case "tags[]":
			body["tags"] = values
		default:
			if len(values) == 1 {
				body[key] = values[0]
			} else if len(values) > 1 {
				body[key] = values
			}
		}
	}

	if rows := additionalDetailRows(params); rows != nil {
		body["additional_details"] = rows
	}

	if files != nil {
		body["files"] = files
	}
	return body
}

// The form carries a row under its own index (setAdditionalDetailParams); a JSON body needs them
// gathered back into objects, in index order.
func additionalDetailRows(params url.Values) []map[string]any {
	var rows []map[string]any
	for key, values := range params {
		match := additionalDetailParamPattern.FindStringSubmatch(key)
		if match == nil || len(values) == 0 {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		for len(rows) <= index {
			rows = append(rows, map[string]any{})
		}
		rows[index][match[2]] = values[0]
	}
	return rows
}
