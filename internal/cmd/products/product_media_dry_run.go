package products

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
)

func productMediaDryRunUploads(media []plannedProductMedia) []dryRunCreateUpload {
	uploads := make([]dryRunCreateUpload, 0, len(media))
	for _, current := range media {
		uploads = append(uploads, dryRunCreateUpload{
			Action:      "direct_upload",
			Kind:        string(current.Kind),
			Path:        current.Path,
			Filename:    current.Filename,
			Size:        current.Size,
			PartSize:    current.Size,
			PartCount:   1,
			ContentType: current.ContentType,
			Checksum:    current.Checksum,
		})
	}
	return uploads
}

func productMediaDryRunRequests(productID string, media []plannedProductMedia) []dryRunCreateRequest {
	requests := make([]dryRunCreateRequest, 0, len(media)*2)
	for i, current := range media {
		requests = append(requests, dryRunCreateRequest{
			Method: http.MethodPost,
			Path:   "/direct_uploads",
			Body: map[string]any{
				"blob": map[string]any{
					"filename":     current.Filename,
					"byte_size":    current.Size,
					"checksum":     current.Checksum,
					"content_type": current.ContentType,
				},
			},
		})
		requests = append(requests, dryRunCreateRequest{
			Method: http.MethodPost,
			Path:   productMediaAttachPath(productID, current.Kind),
			Body: map[string]any{
				"signed_blob_id": dryRunSignedBlobPlaceholder(current.Kind, i),
			},
		})
	}
	return requests
}

func dryRunSignedBlobPlaceholder(kind productMediaKind, index int) string {
	return fmt.Sprintf("<signed_blob:%s:%d>", kind, index)
}

func renderProductMediaDryRunPlain(opts cmdutil.Options, media plannedProductMedia) error {
	return output.PrintPlain(opts.Out(), [][]string{{
		"direct_upload",
		string(media.Kind),
		media.Path,
		media.Filename,
		strconv.FormatInt(media.Size, 10),
		media.ContentType,
	}})
}

func renderProductMediaDryRun(opts cmdutil.Options, media plannedProductMedia) error {
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": direct upload "+media.Path); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Filename: %s\n", media.Filename); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Content type: %s\n", media.ContentType); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Size: %s\n", uploadui.HumanBytes(media.Size))
}

func renderStandaloneProductMediaDryRun(opts cmdutil.Options, media []plannedProductMedia, requests []dryRunCreateRequest) error {
	if opts.UsesJSONOutput() {
		payload := dryRunCreatePayload{
			DryRun:  true,
			Uploads: productMediaDryRunUploads(media),
		}
		if len(requests) > 0 {
			payload.Request = requests[0]
			payload.FollowUpRequests = requests[1:]
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}
	if opts.PlainOutput {
		for _, planned := range media {
			if err := renderProductMediaDryRunPlain(opts, planned); err != nil {
				return err
			}
		}
		return renderDryRunRequestsPlain(opts, requests)
	}
	for _, planned := range media {
		if err := renderProductMediaDryRun(opts, planned); err != nil {
			return err
		}
	}
	return renderDryRunRequestsHuman(opts, requests)
}

func renderDryRunRequestsPlain(opts cmdutil.Options, requests []dryRunCreateRequest) error {
	for _, request := range requests {
		data, err := json.Marshal(request.Body)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		if err := output.PrintPlain(opts.Out(), [][]string{{request.Method, request.Path, string(data)}}); err != nil {
			return err
		}
	}
	return nil
}

func renderDryRunRequestsHuman(opts cmdutil.Options, requests []dryRunCreateRequest) error {
	style := opts.Style()
	for _, request := range requests {
		if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": "+request.Method+" "+request.Path); err != nil {
			return err
		}
		data, err := json.MarshalIndent(request.Body, "", "  ")
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		if err := output.Writeln(opts.Out(), string(data)); err != nil {
			return err
		}
	}
	return nil
}
