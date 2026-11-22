package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/spf13/cobra"
)

// maxManifestBytes caps the size of a recovery manifest read from a file or
// stdin. A recovery manifest is a small JSON control document; anything
// larger is either a mistake (the wrong file piped in) or a non-terminating
// producer that would otherwise hang the CLI or exhaust memory.
const maxManifestBytes = 2 * 1024 * 1024 // 2 MB

// completeManifest mirrors the Recovery section of a failed upload's JSON
// error envelope, so the user can pipe `--json` error output straight back in.
type completeManifest struct {
	// FileURL is the canonical URL the server would assign if it already
	// committed. When non-empty, the upload may already be finalized and a
	// blind re-finalize would create a duplicate — we gate on confirmation.
	FileURL        string                 `json:"file_url,omitempty"`
	UploadID       string                 `json:"upload_id"`
	Key            string                 `json:"key"`
	CompletedParts []completeManifestPart `json:"completed_parts"`
}

type completeManifestPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

type dryRunCompleteRequest struct {
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body"`
}

type dryRunCompletePayload struct {
	DryRun  bool                  `json:"dry_run"`
	Request dryRunCompleteRequest `json:"request"`
}

func newCompleteCmd() *cobra.Command {
	var recoveryPath string
	c := &cobra.Command{
		Use:   "complete",
		Short: "Finalize an upload that hit an ambiguous /files/complete failure",
		Args:  cmdutil.ExactArgs(0),
		Long: "Replay /files/complete with a saved recovery manifest so an upload " +
			"that left `upload` with a complete_state_unknown error can be " +
			"finalized without re-uploading any bytes.\n\n" +
			"The manifest is the `.error.recovery` object from a failed --json " +
			"upload, or a plain {\"upload_id\": ..., \"key\": ..., \"completed_parts\": [...]} " +
			"JSON document. Pass `-` to read from stdin.\n\n" +
			"If the manifest includes a file_url, the upload may have already " +
			"committed on the server; the command will prompt for confirmation " +
			"(or require --yes) before replaying /files/complete, since a blind " +
			"re-finalize can duplicate the attachment.",
		Example: `  gumroad files upload ./pack.zip --json > err.json
  jq '.error.recovery' err.json > recovery.json
  gumroad files complete --recovery recovery.json
  # When piping on stdin, --yes is required because the manifest consumes the
  # only input stream and the duplicate-risk prompt has nowhere to read from:
  jq '.error.recovery' err.json | gumroad files complete --recovery - --yes`,
		RunE: func(c *cobra.Command, _ []string) error {
			opts := cmdutil.OptionsFrom(c)
			if recoveryPath == "" {
				return cmdutil.MissingFlagError(c, "--recovery")
			}
			// --recovery - consumes stdin for the manifest, leaving no
			// stream for the duplicate-risk confirmation prompt below.
			// Require --yes (or --dry-run) in that case so the user has
			// explicitly acknowledged the retry-may-duplicate semantics.
			if recoveryPath == "-" && !opts.Yes && !opts.DryRun {
				return cmdutil.UsageErrorf(c,
					"--recovery - consumes stdin for the manifest, so the duplicate-risk confirmation has nowhere to read from. Save the manifest to a file, or pass --yes to acknowledge the replay risk up front.")
			}

			manifest, err := readCompleteManifest(recoveryPath, opts.In())
			if err != nil {
				return err
			}
			if manifest.UploadID == "" || manifest.Key == "" {
				return cmdutil.UsageErrorf(c, "recovery manifest missing upload_id or key")
			}
			if len(manifest.CompletedParts) == 0 {
				return cmdutil.UsageErrorf(c, "recovery manifest has no completed_parts")
			}
			// Validate every part up front, before confirmation, so
			// malformed manifests fail fast and do not prompt the caller to
			// acknowledge a replay that cannot succeed. The uploader only
			// ever produces contiguous part_numbers 1..N, so anything else
			// would re-finalize the wrong bytes order.
			for i, p := range manifest.CompletedParts {
				if p.ETag == "" {
					return cmdutil.UsageErrorf(c, "completed_parts[%d] missing etag", i)
				}
				if p.PartNumber != i+1 {
					return cmdutil.UsageErrorf(c,
						"completed_parts[%d].part_number = %d; expected %d (manifests must list parts contiguously from 1)",
						i, p.PartNumber, i+1)
				}
			}

			// Every complete_state_unknown replay is unsafe by default:
			// /files/complete is the point where the server may have already
			// committed, and the presign response's file_url is optional —
			// its absence does not prove the upload didn't commit. Confirm
			// unconditionally so no recovery path finalizes without the
			// caller acknowledging the duplicate risk. --yes skips the
			// prompt; --no-input returns the usual blocking error.
			prompt := fmt.Sprintf(
				"Re-finalize multipart upload %s? The server may have already committed it, so a replay can create a duplicate.",
				manifest.UploadID,
			)
			if manifest.FileURL != "" {
				prompt = fmt.Sprintf(
					"Re-finalize multipart upload %s? The recovery manifest carries file_url=%s — the server may have already committed this upload, so a replay can create a duplicate.",
					manifest.UploadID, manifest.FileURL,
				)
			}
			ok, err := cmdutil.ConfirmAction(opts, prompt)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "finalize multipart upload "+manifest.UploadID, manifest.UploadID)
			}

			body := buildCompleteJSONBody(manifest)
			if opts.DryRun {
				return renderCompleteDryRun(opts, "/files/complete", body)
			}

			return runComplete(opts, manifest, body)
		},
	}
	c.Flags().StringVar(&recoveryPath, "recovery", "", "Path to the recovery manifest JSON (use `-` for stdin) (required)")
	return c
}

func buildCompleteJSONBody(manifest completeManifest) map[string]any {
	parts := make([]map[string]any, len(manifest.CompletedParts))
	for i, p := range manifest.CompletedParts {
		parts[i] = map[string]any{
			"part_number": p.PartNumber,
			"etag":        p.ETag,
		}
	}
	return map[string]any{
		"upload_id": manifest.UploadID,
		"key":       manifest.Key,
		"parts":     parts,
	}
}

func renderCompleteDryRun(opts cmdutil.Options, path string, body map[string]any) error {
	switch {
	case opts.UsesJSONOutput():
		payload := dryRunCompletePayload{
			DryRun: true,
			Request: dryRunCompleteRequest{
				Method: http.MethodPost,
				Path:   path,
				Body:   body,
			},
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	case opts.PlainOutput:
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintPlain(opts.Out(), [][]string{{http.MethodPost, path, string(data)}})
	default:
		style := opts.Style()
		if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": "+http.MethodPost+" "+path); err != nil {
			return err
		}
		data, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.Writeln(opts.Out(), string(data))
	}
}

// runComplete calls /files/complete and prints the returned file_url in the
// same shape `files upload` does, so both success paths are interchangeable
// for scripts that pipe to `jq '.file_url'`. Ambiguous failures are wrapped
// with *upload.UnknownStateError so the caller keeps the recovery manifest
// on a retry path — otherwise a single 5xx during replay would strand the
// upload with no further safe reconciliation option.
func runComplete(opts cmdutil.Options, manifest completeManifest, body map[string]any) error {
	token, err := config.Token()
	if err != nil {
		return err
	}
	client := cmdutil.NewAPIClient(opts, token)

	var sp *output.Spinner
	if cmdutil.ShouldShowSpinner(opts) {
		sp = output.NewSpinnerTo("Finalizing multipart upload...", opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	data, err := client.PostJSONWithContext(opts.Context, "/files/complete", body)
	if err != nil {
		return handleCompleteFailure(err, manifest)
	}
	if sp != nil {
		sp.Stop()
	}

	var resp struct {
		FileURL string `json:"file_url"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return handleCompleteFailure(fmt.Errorf("could not parse /files/complete response: %w", err), manifest)
	}
	if resp.FileURL == "" {
		return handleCompleteFailure(errors.New("/files/complete response missing file_url"), manifest)
	}
	return renderFileURL(opts, resp.FileURL)
}

// handleCompleteFailure wraps a /files/complete failure with the recovery
// manifest so the caller retains actionable state:
//   - Ambiguous (5xx, 408, 409, 429, transport, parse, or HTTP 400 with
//     error_code=multipart_upload_not_found): *upload.UnknownStateError.
//     The manifest remains available for reconciliation.
//   - Definitive (other 4xx, or HTTP-200-with-success-false): *CompleteRejectedError
//     — preserves upload_id/key so the caller can run `gumroad files abort`
//     after verifying that abort is actually the right next step. Unlike
//     upload.Upload, this recovery command does not auto-abort: the manifest
//     here is external input, and common causes of a 4xx here (wrong seller
//     token, slightly wrong manifest) are recoverable with the SAME parts
//     still present on S3. Auto-aborting would destroy that state.
func handleCompleteFailure(err error, manifest completeManifest) error {
	parts := make([]upload.CompletedPart, len(manifest.CompletedParts))
	for i, p := range manifest.CompletedParts {
		parts[i] = upload.CompletedPart{PartNumber: p.PartNumber, ETag: p.ETag}
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && isDefinitiveCompleteRejection(apiErr.StatusCode) &&
		(apiErr.StatusCode != http.StatusBadRequest || apiErr.Code != "multipart_upload_not_found") {
		return &CompleteRejectedError{
			FileURL:        manifest.FileURL,
			UploadID:       manifest.UploadID,
			Key:            manifest.Key,
			CompletedParts: parts,
			Cause:          err,
		}
	}
	return &upload.UnknownStateError{
		FileURL:        manifest.FileURL,
		UploadID:       manifest.UploadID,
		Key:            manifest.Key,
		CompletedParts: parts,
		Cause:          err,
	}
}

// CompleteRejectedError wraps a definitive /files/complete rejection with
// the full recovery manifest so callers can either correct a caller-side
// mistake (wrong token, malformed manifest) and re-run `gumroad files
// complete`, or reclaim the orphaned multipart upload via `gumroad files
// abort`. It delegates its Error() to the cause, so the API error message
// still reaches the user; the manifest survives as structured fields for
// JSON consumers and human recovery output. Keeping CompletedParts is
// critical for the stdin-piped recovery flow, where a 4xx on replay
// otherwise consumes the only copy of the manifest.
type CompleteRejectedError struct {
	FileURL        string
	UploadID       string
	Key            string
	CompletedParts []upload.CompletedPart
	Cause          error
}

func (e *CompleteRejectedError) Error() string { return e.Cause.Error() }
func (e *CompleteRejectedError) Unwrap() error { return e.Cause }

// isDefinitiveCompleteRejection mirrors internal/upload's classification:
// transient 4xx (408, 409, 429) and any 5xx remain ambiguous; everything
// else from a 4xx API rejection is a definitive no-commit.
func isDefinitiveCompleteRejection(status int) bool {
	if status >= 500 {
		return false
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return false
	}
	return true
}

func readCompleteManifest(path string, stdin io.Reader) (completeManifest, error) {
	var r io.Reader
	if path == "-" {
		r = stdin
	} else {
		// Stat-before-open: os.Open on a FIFO or device blocks the
		// process indefinitely until a writer appears, which would
		// defeat the size cap below and leave the recovery command
		// hung. Mirrors the guard the upload package uses on its own
		// file-open path.
		info, err := os.Stat(path)
		if err != nil {
			return completeManifest{}, fmt.Errorf("could not open recovery manifest: %w", err)
		}
		if !info.Mode().IsRegular() {
			return completeManifest{}, fmt.Errorf("recovery manifest %s is not a regular file", path)
		}
		f, err := os.Open(path)
		if err != nil {
			return completeManifest{}, fmt.Errorf("could not open recovery manifest: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	// Read one byte past the cap so we can distinguish "exactly at limit"
	// from "too large". Recovery manifests are small control JSON; a
	// producer that outpaces that is almost certainly a mistake (wrong
	// stream piped in) and should fail fast rather than consume memory.
	data, err := io.ReadAll(io.LimitReader(r, maxManifestBytes+1))
	if err != nil {
		return completeManifest{}, fmt.Errorf("could not read recovery manifest: %w", err)
	}
	if int64(len(data)) > maxManifestBytes {
		return completeManifest{}, fmt.Errorf("recovery manifest exceeds %d bytes; provide only the .error.recovery JSON object", maxManifestBytes)
	}
	var manifest completeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return completeManifest{}, fmt.Errorf("could not parse recovery manifest: %w", err)
	}
	return manifest, nil
}
