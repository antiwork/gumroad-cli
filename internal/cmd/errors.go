package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmd/files"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type commandErrorEnvelope struct {
	Success bool               `json:"success"`
	Error   commandErrorDetail `json:"error"`
}

type commandErrorDetail struct {
	Type       string              `json:"type"`
	Code       string              `json:"code,omitempty"`
	Message    string              `json:"message"`
	Hint       string              `json:"hint,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Recovery   *uploadRecoveryInfo `json:"recovery,omitempty"`
}

// uploadRecoveryInfo carries the handles a caller needs to reconcile an
// ambiguous multipart upload: whether the file committed (file_url), the
// identifiers needed to retry /files/complete or /files/abort (upload_id, key),
// and the parts already successfully PUT to S3 (so the caller doesn't re-upload
// bytes on retry).
type uploadRecoveryInfo struct {
	FileURL        string                `json:"file_url,omitempty"`
	UploadID       string                `json:"upload_id,omitempty"`
	Key            string                `json:"key,omitempty"`
	CompletedParts []uploadCompletedPart `json:"completed_parts,omitempty"`
}

type uploadCompletedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

func printStructuredCommandError(w io.Writer, err error) error {
	payload := commandErrorEnvelope{
		Success: false,
		Error:   classifyCommandError(err),
	}

	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return fmt.Errorf("could not encode error output: %w", marshalErr)
	}

	return output.PrintJSON(w, data, "")
}

func classifyCommandError(err error) commandErrorDetail {
	if err == nil {
		return commandErrorDetail{
			Type:    "internal_error",
			Code:    "unknown_error",
			Message: "unknown error",
		}
	}

	// Classify the primary cause with cleanup branches stripped.
	// CleanupFailedError.Unwrap() exposes the abort call's cause (typically
	// an *api.APIError from /files/abort). Without stripping, a joined
	// upload error would surface the abort's APIError as the primary
	// classification, hiding the cleanup_failed signal the caller needs to
	// reclaim orphaned storage.
	detail := classifyPrimaryCause(primaryCause(err))

	var cleanup *upload.CleanupFailedError
	if errors.As(err, &cleanup) {
		// If the primary had no better classification than "internal_error"
		// (plain errors from part-upload failures, context cancellation),
		// the cleanup failure is the most actionable signal — surface it
		// as the primary code so automation notices the orphan.
		if detail.Type == "internal_error" {
			detail = uploadCleanupFailedDetail(err, cleanup)
		}
		// A CleanupFailedError joined alongside a better-classified primary
		// (API error, auth error, usage error) must not erase that
		// classification. Attach orphan handles as a secondary Recovery
		// signal unless the primary already supplied one (UnknownStateError
		// carries its own manifest).
		if detail.Recovery == nil {
			detail.Recovery = &uploadRecoveryInfo{
				UploadID: cleanup.UploadID,
				Key:      cleanup.Key,
			}
		}
	}

	return detail
}

// primaryCause returns err with any *upload.CleanupFailedError branches of a
// multi-unwrap (errors.Join) tree removed so the primary failure is not
// shadowed by the abort call's cause. Returns err unchanged if stripping
// leaves nothing behind (pure cleanup-only failure) or if err is not a
// multi-unwrap.
func primaryCause(err error) error {
	type multiUnwrap interface{ Unwrap() []error }
	mu, ok := err.(multiUnwrap)
	if !ok {
		return err
	}
	var kept []error
	for _, inner := range mu.Unwrap() {
		if _, isCleanup := inner.(*upload.CleanupFailedError); isCleanup {
			continue
		}
		kept = append(kept, inner)
	}
	switch len(kept) {
	case 0:
		return err
	case 1:
		return kept[0]
	default:
		return errors.Join(kept...)
	}
}

func classifyPrimaryCause(err error) commandErrorDetail {
	var usageErr *cmdutil.UsageError
	var apiErr *api.APIError
	var unknownState *upload.UnknownStateError
	var cleanupFailed *upload.CleanupFailedError
	var rejected *files.CompleteRejectedError
	switch {
	case errors.As(err, &usageErr):
		return invalidInputErrorDetail(usageErr.Error())
	case errors.As(err, &unknownState):
		return uploadIncompleteDetail(err, unknownState, cleanupFailedFrom(err))
	// ErrPresignExpired is a sentinel distinct from UnknownStateError: the
	// server never committed, so a retry-the-whole-upload is safe (no
	// duplicate risk). Classify it explicitly so automation distinguishes
	// "restart" from "reconcile".
	case errors.Is(err, upload.ErrPresignExpired):
		return commandErrorDetail{
			Type:    "upload_error",
			Code:    "presign_expired",
			Message: err.Error(),
			Hint:    "Presigned upload URLs expired mid-flight. Re-run `gumroad files upload` from scratch — a fresh presign round covers all parts.",
		}
	// Match CompleteRejectedError before api.APIError so rejected replays
	// become first-class upload failures. But when the underlying cause
	// is an auth/access error (401/403), keep the auth classification so
	// callers are told to refresh credentials rather than treat it as a
	// pure upload-recovery problem — the orphan handles are still
	// attached as recovery metadata for later cleanup.
	case errors.As(err, &rejected):
		return completeRejectedClassification(err, rejected)
	// Match CleanupFailedError before api.APIError: when only a cleanup
	// error is in scope (because primaryCause stripped the primary tree),
	// we want cleanup_failed classification, not the abort call's APIError
	// exposed via CleanupFailedError.Unwrap().
	case errors.As(err, &cleanupFailed):
		return uploadCleanupFailedDetail(err, cleanupFailed)
	case errors.As(err, &apiErr):
		return commandErrorDetail{
			Type:       "api_error",
			Code:       apiErrorCode(apiErr.StatusCode),
			Message:    apiErr.Error(),
			Hint:       apiErr.GetHint(),
			StatusCode: apiErr.StatusCode,
		}
	case errors.Is(err, config.ErrNotAuthenticated), errors.Is(err, api.ErrNotAuthenticated):
		hint := api.HintRunAuthLogin
		if strings.Contains(err.Error(), "gumroad auth login") {
			hint = ""
		}
		return commandErrorDetail{
			Type:    "auth_error",
			Code:    "not_authenticated",
			Message: err.Error(),
			Hint:    hint,
		}
	case errors.Is(err, prompt.ErrConfirmationNoInput), errors.Is(err, prompt.ErrConfirmationNonInteractive):
		return invalidInputErrorDetail(err.Error())
	case isLikelyJQError(err):
		return commandErrorDetail{
			Type:    "usage_error",
			Code:    "invalid_jq",
			Message: err.Error(),
		}
	case isLikelyUsageError(err):
		return invalidInputErrorDetail(err.Error())
	default:
		return commandErrorDetail{
			Type:    "internal_error",
			Code:    "internal_error",
			Message: err.Error(),
		}
	}
}

func invalidInputErrorDetail(message string) commandErrorDetail {
	return commandErrorDetail{
		Type:    "usage_error",
		Code:    "invalid_input",
		Message: message,
	}
}

// cleanupFailedFrom extracts an optional joined CleanupFailedError so the
// recovery payload carries both state-unknown and abort-orphan identifiers
// when they co-occur.
func cleanupFailedFrom(err error) *upload.CleanupFailedError {
	var cleanup *upload.CleanupFailedError
	if errors.As(err, &cleanup) {
		return cleanup
	}
	return nil
}

func uploadIncompleteDetail(err error, state *upload.UnknownStateError, cleanup *upload.CleanupFailedError) commandErrorDetail {
	recovery := &uploadRecoveryInfo{
		FileURL:  state.FileURL,
		UploadID: state.UploadID,
		Key:      state.Key,
	}
	if len(state.CompletedParts) > 0 {
		recovery.CompletedParts = make([]uploadCompletedPart, len(state.CompletedParts))
		for i, p := range state.CompletedParts {
			recovery.CompletedParts[i] = uploadCompletedPart{PartNumber: p.PartNumber, ETag: p.ETag}
		}
	}
	if cleanup != nil {
		if recovery.UploadID == "" {
			recovery.UploadID = cleanup.UploadID
		}
		if recovery.Key == "" {
			recovery.Key = cleanup.Key
		}
	}
	// The recovery's file_url is a non-authoritative hint from the presign
	// response: the upload package documents the authoritative URL as the
	// one returned by /files/complete, so a matching file_url is not
	// proof of commit. Speak in terms of the actions the caller has: use
	// `files complete` to replay finalize (safe — the same parts remain),
	// or `files abort` to reclaim storage if they decide to give up.
	hint := "Do not retry blindly — a retry may create a duplicate. Finalize with `gumroad files complete --recovery <manifest>` to commit without re-uploading, or reclaim storage with `gumroad files abort --upload-id <id> --key <key>`."
	return commandErrorDetail{
		Type:     "upload_error",
		Code:     "complete_state_unknown",
		Message:  err.Error(),
		Hint:     hint,
		Recovery: recovery,
	}
}

// printUploadRecovery emits any recovery handles carried by multipart-upload
// errors so a human operator can reconcile state without re-uploading blindly.
func printUploadRecovery(w io.Writer, style output.Styler, err error) {
	var state *upload.UnknownStateError
	var cleanup *upload.CleanupFailedError
	var rejected *files.CompleteRejectedError
	hasState := errors.As(err, &state)
	hasCleanup := errors.As(err, &cleanup)
	hasRejected := errors.As(err, &rejected)
	if !hasState && !hasCleanup && !hasRejected {
		return
	}

	uploadID, key := resolveOrphanHandles(state, cleanup, rejected, hasState, hasCleanup, hasRejected)

	fmt.Fprintln(w, style.Dim("Recovery:"))
	// The upload package says this URL is the presign-side hint and is
	// not authoritative; avoid phrasing that implies it is.
	if hasState && state.FileURL != "" {
		fmt.Fprintln(w, style.Dim("  file_url:  "+state.FileURL)+style.Dim("  (non-authoritative presign hint)"))
	} else if hasRejected && rejected.FileURL != "" {
		fmt.Fprintln(w, style.Dim("  file_url:  "+rejected.FileURL)+style.Dim("  (non-authoritative presign hint)"))
	}
	if uploadID != "" {
		fmt.Fprintln(w, style.Dim("  upload_id: "+uploadID))
	}
	if key != "" {
		fmt.Fprintln(w, style.Dim("  key:       "+key))
	}
	parts := resolveRecoveryParts(state, rejected, hasState, hasRejected)
	if n := len(parts); n > 0 {
		fmt.Fprintln(w, style.Dim(fmt.Sprintf("  completed_parts: %d uploaded (re-finalize with `gumroad files complete --recovery <manifest>` to avoid re-uploading):", n)))
		for _, p := range parts {
			fmt.Fprintln(w, style.Dim(fmt.Sprintf("    part_number=%d etag=%s", p.PartNumber, p.ETag)))
		}
	}
	if hasCleanup && (uploadID != "" || key != "") {
		fmt.Fprintln(w, style.Dim("  cleanup_failed: multipart left orphaned on S3; reclaim with `gumroad files abort --upload-id <id> --key <key>`"))
	}
	if hasRejected && (uploadID != "" || key != "") {
		fmt.Fprintln(w, style.Dim("  finalize_rejected: /files/complete refused this manifest. If the rejection looks recoverable (wrong token, fixable manifest), correct it and re-run `gumroad files complete`; otherwise reclaim the orphan with `gumroad files abort --upload-id <id> --key <key>`."))
	}
}

// resolveRecoveryParts returns the completed-parts manifest carried by the
// error, preferring the UnknownStateError copy (the original uploader
// produced it) over the CompleteRejectedError copy (echoed back from the
// manifest the user supplied).
func resolveRecoveryParts(state *upload.UnknownStateError, rejected *files.CompleteRejectedError, hasState, hasRejected bool) []upload.CompletedPart {
	if hasState && len(state.CompletedParts) > 0 {
		return state.CompletedParts
	}
	if hasRejected {
		return rejected.CompletedParts
	}
	return nil
}

// resolveOrphanHandles returns the upload_id/key pair to print, preferring
// whichever source carries the primary failure identifiers.
func resolveOrphanHandles(state *upload.UnknownStateError, cleanup *upload.CleanupFailedError, rejected *files.CompleteRejectedError, hasState, hasCleanup, hasRejected bool) (uploadID, key string) {
	if hasState {
		uploadID = state.UploadID
		key = state.Key
	}
	if hasRejected {
		if uploadID == "" {
			uploadID = rejected.UploadID
		}
		if key == "" {
			key = rejected.Key
		}
	}
	if hasCleanup {
		if uploadID == "" {
			uploadID = cleanup.UploadID
		}
		if key == "" {
			key = cleanup.Key
		}
	}
	return uploadID, key
}

func completeRejectedClassification(err error, rejected *files.CompleteRejectedError) commandErrorDetail {
	recovery := &uploadRecoveryInfo{
		FileURL:  rejected.FileURL,
		UploadID: rejected.UploadID,
		Key:      rejected.Key,
	}
	if len(rejected.CompletedParts) > 0 {
		recovery.CompletedParts = make([]uploadCompletedPart, len(rejected.CompletedParts))
		for i, p := range rejected.CompletedParts {
			recovery.CompletedParts[i] = uploadCompletedPart{PartNumber: p.PartNumber, ETag: p.ETag}
		}
	}

	// When the underlying cause is an auth or access-denied API error,
	// preserve that classification. The primary remediation is refreshing
	// credentials, not reconciling the upload — the recovery handles are
	// attached as supplemental metadata for later cleanup.
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		code := apiErrorCode(apiErr.StatusCode)
		if code == "not_authenticated" || code == "access_denied" {
			return commandErrorDetail{
				Type:       "api_error",
				Code:       code,
				Message:    apiErr.Error(),
				Hint:       apiErr.GetHint(),
				StatusCode: apiErr.StatusCode,
				Recovery:   recovery,
			}
		}
	}

	detail := commandErrorDetail{
		Type:     "upload_error",
		Code:     "complete_rejected",
		Message:  err.Error(),
		Hint:     "`/files/complete` refused the manifest on replay. If the rejection is recoverable (fixable manifest), correct it and re-run `gumroad files complete`. Otherwise reclaim storage with `gumroad files abort --upload-id <id> --key <key>`.",
		Recovery: recovery,
	}
	if apiErr != nil {
		detail.StatusCode = apiErr.StatusCode
	}
	return detail
}

func uploadCleanupFailedDetail(err error, cleanup *upload.CleanupFailedError) commandErrorDetail {
	return commandErrorDetail{
		Type:    "upload_error",
		Code:    "cleanup_failed",
		Message: err.Error(),
		Hint:    "A multipart upload was left orphaned on S3. Reclaim it with `gumroad files abort --upload-id <id> --key <key>`.",
		Recovery: &uploadRecoveryInfo{
			UploadID: cleanup.UploadID,
			Key:      cleanup.Key,
		},
	}
}

func apiErrorCode(statusCode int) string {
	switch statusCode {
	case 401:
		return "not_authenticated"
	case 403:
		return "access_denied"
	case 404:
		return "not_found"
	case 429:
		return "rate_limited"
	default:
		return "api_error"
	}
}

func structuredOutputRequested(cmd *cobra.Command) bool {
	if structuredOutputRequestedFromCommand(cmd) {
		return true
	}
	return structuredOutputRequestedInArgs(os.Args[1:])
}

func structuredOutputRequestedFromCommand(cmd *cobra.Command) bool {
	opts := cmdutil.OptionsFrom(cmd)
	if opts.UsesJSONOutput() {
		return true
	}
	if cmd == nil {
		return false
	}

	return structuredOutputRequestedInFlagSet(cmd.Flags()) ||
		structuredOutputRequestedInFlagSet(cmd.PersistentFlags())
}

func structuredOutputRequestedInFlagSet(flags *pflag.FlagSet) bool {
	if flags == nil {
		return false
	}

	jsonOutput, err := flags.GetBool("json")
	if err == nil && jsonOutput {
		return true
	}

	jqExpr, err := flags.GetString("jq")
	return err == nil && jqExpr != ""
}

func structuredOutputRequestedInArgs(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--json":
			return true
		case strings.HasPrefix(arg, "--json="):
			value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--json="))
			if err == nil && value {
				return true
			}
		case arg == "--jq":
			return true
		case strings.HasPrefix(arg, "--jq="):
			return true
		}
	}

	return false
}

func isLikelyUsageError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.HasPrefix(message, "unknown command ") ||
		strings.HasPrefix(message, "unknown flag: ") ||
		strings.HasPrefix(message, "unknown shorthand flag: ") ||
		strings.Contains(message, " requires at least") ||
		strings.Contains(message, "flag needs an argument")
}

func isLikelyJQError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.HasPrefix(message, "invalid jq expression:") ||
		strings.HasPrefix(message, "jq error:")
}
