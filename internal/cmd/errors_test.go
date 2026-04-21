package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

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

func TestPrintStructuredCommandError(t *testing.T) {
	var buf bytes.Buffer
	err := printStructuredCommandError(&buf, &api.APIError{StatusCode: 403, Message: "Access denied: scope"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload commandErrorEnvelope
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %v with %q", err, buf.String())
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if payload.Error.Type != "api_error" || payload.Error.Code != "access_denied" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestPrintStructuredCommandError_WithHint(t *testing.T) {
	var buf bytes.Buffer
	err := printStructuredCommandError(&buf, &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload commandErrorEnvelope
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %v with %q", err, buf.String())
	}
	if payload.Error.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", payload.Error.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_APIWithHint(t *testing.T) {
	detail := classifyCommandError(&api.APIError{StatusCode: 404, Message: "Resource not found.", Hint: "Check the resource ID and try again."})
	if detail.Hint != "Check the resource ID and try again." {
		t.Errorf("got hint %q", detail.Hint)
	}
}

func TestClassifyCommandError_AuthHint(t *testing.T) {
	detail := classifyCommandError(config.ErrNotAuthenticated)
	if detail.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", detail.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_WrappedAPIError(t *testing.T) {
	wrapped := fmt.Errorf("invalid token: %w", &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin})
	detail := classifyCommandError(wrapped)
	if detail.Type != "api_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", detail.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_WrappedConfigAuth(t *testing.T) {
	wrapped := fmt.Errorf("setup failed: %w", config.ErrNotAuthenticated)
	detail := classifyCommandError(wrapped)
	if detail.Type != "auth_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", detail.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_ConfigAuthWithRemediationInMessage(t *testing.T) {
	// Simulates the real error from config.ResolveToken which already embeds remediation.
	wrapped := fmt.Errorf("%w. Run `gumroad auth login` first or set `GUMROAD_ACCESS_TOKEN`", config.ErrNotAuthenticated)
	detail := classifyCommandError(wrapped)
	if detail.Type != "auth_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Hint != "" {
		t.Errorf("expected empty hint when message already contains remediation, got %q", detail.Hint)
	}
}

func TestClassifyCommandError_Nil(t *testing.T) {
	detail := classifyCommandError(nil)
	if detail.Type != "internal_error" || detail.Code != "unknown_error" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_Usage(t *testing.T) {
	cmd := &cobra.Command{Use: "gumroad user"}
	detail := classifyCommandError(cmdutil.UsageErrorf(cmd, "bad input"))
	if detail.Type != "usage_error" || detail.Code != "invalid_input" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_API(t *testing.T) {
	detail := classifyCommandError(&api.APIError{StatusCode: 429, Message: "Rate limited"})
	if detail.Type != "api_error" || detail.Code != "rate_limited" || detail.StatusCode != 429 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_Auth(t *testing.T) {
	detail := classifyCommandError(config.ErrNotAuthenticated)
	if detail.Type != "auth_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_JQ(t *testing.T) {
	detail := classifyCommandError(errors.New("invalid jq expression: bad token"))
	if detail.Type != "usage_error" || detail.Code != "invalid_jq" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_LikelyUsageError(t *testing.T) {
	detail := classifyCommandError(errors.New("unknown command \"bad\" for \"gumroad\""))
	if detail.Type != "usage_error" || detail.Code != "invalid_input" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_ConfirmationUsageError(t *testing.T) {
	detail := classifyCommandError(fmt.Errorf("%w. Use --yes to skip confirmation", prompt.ErrConfirmationNoInput))
	if detail.Type != "usage_error" || detail.Code != "invalid_input" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestAPIErrorCode(t *testing.T) {
	for statusCode, want := range map[int]string{
		401: "not_authenticated",
		403: "access_denied",
		404: "not_found",
		429: "rate_limited",
		500: "api_error",
	} {
		if got := apiErrorCode(statusCode); got != want {
			t.Fatalf("status %d: got %q, want %q", statusCode, got, want)
		}
	}
}

func TestStructuredOutputRequestedInFlagSet(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("json", false, "")
	flags.String("jq", "", "")

	if structuredOutputRequestedInFlagSet(flags) {
		t.Fatal("expected empty flag set to be false")
	}
	if err := flags.Set("json", "true"); err != nil {
		t.Fatalf("Set(json) failed: %v", err)
	}
	if !structuredOutputRequestedInFlagSet(flags) {
		t.Fatal("expected json=true to request structured output")
	}

	flags = pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("json", false, "")
	flags.String("jq", "", "")
	if err := flags.Set("jq", ".user.email"); err != nil {
		t.Fatalf("Set(jq) failed: %v", err)
	}
	if !structuredOutputRequestedInFlagSet(flags) {
		t.Fatal("expected jq to request structured output")
	}
}

func TestStructuredOutputRequestedFromCommandWithoutContext(t *testing.T) {
	cmd := stubCommand(nil)
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("jq", "", "")

	if structuredOutputRequestedFromCommand(cmd) {
		t.Fatal("expected command without flags to be false")
	}
	if err := cmd.Flags().Set("jq", ".user.email"); err != nil {
		t.Fatalf("Set(jq) failed: %v", err)
	}
	if !structuredOutputRequestedFromCommand(cmd) {
		t.Fatal("expected jq flag to request structured output")
	}
}

func TestLikelyErrorHelpers(t *testing.T) {
	if !isLikelyUsageError(errors.New("flag needs an argument: --product")) {
		t.Fatal("expected usage-like error to be detected")
	}
	if isLikelyUsageError(errors.New("plain error")) {
		t.Fatal("did not expect plain error to be usage-like")
	}

	if !isLikelyJQError(errors.New("jq error: bad path")) {
		t.Fatal("expected jq-like error to be detected")
	}
	if isLikelyJQError(errors.New("plain error")) {
		t.Fatal("did not expect plain error to be jq-like")
	}
}

func TestPrintStructuredCommandError_MarshalFallback(t *testing.T) {
	err := printStructuredCommandError(bytes.NewBuffer(nil), errors.New(strings.Repeat("x", 8)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyCommandError_UploadUnknownState_CarriesRecovery(t *testing.T) {
	state := &upload.UnknownStateError{
		FileURL:  "https://example.com/attachments/u/k/file.bin",
		UploadID: "up-1",
		Key:      "attachments/u/k/original/file.bin",
		CompletedParts: []upload.CompletedPart{
			{PartNumber: 1, ETag: "etag-1"},
			{PartNumber: 2, ETag: "etag-2"},
		},
		Cause: errors.New("502 Bad Gateway"),
	}
	detail := classifyCommandError(state)
	if detail.Type != "upload_error" || detail.Code != "complete_state_unknown" {
		t.Fatalf("type/code = %q/%q", detail.Type, detail.Code)
	}
	if detail.Recovery == nil {
		t.Fatal("expected Recovery to be populated")
	}
	if detail.Recovery.FileURL != state.FileURL || detail.Recovery.UploadID != "up-1" || detail.Recovery.Key != state.Key {
		t.Errorf("recovery handles = %+v", detail.Recovery)
	}
	if len(detail.Recovery.CompletedParts) != 2 {
		t.Errorf("completed parts = %d, want 2", len(detail.Recovery.CompletedParts))
	}
	if detail.Hint == "" {
		t.Error("expected human-facing hint about avoiding blind retry")
	}
}

func TestClassifyCommandError_UploadCleanupFailed_CarriesOrphanHandles(t *testing.T) {
	cleanup := &upload.CleanupFailedError{
		UploadID: "up-2",
		Key:      "attachments/u/k/original/file2.bin",
		Cause:    errors.New("abort 500"),
	}
	detail := classifyCommandError(cleanup)
	if detail.Type != "upload_error" || detail.Code != "cleanup_failed" {
		t.Fatalf("type/code = %q/%q", detail.Type, detail.Code)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-2" || detail.Recovery.Key != cleanup.Key {
		t.Errorf("recovery = %+v", detail.Recovery)
	}
}

func TestClassifyCommandError_UnknownStateJoinedWithCleanup_MergesHandles(t *testing.T) {
	state := &upload.UnknownStateError{
		UploadID: "up-3",
		Key:      "attachments/u/k/original/file3.bin",
		Cause:    errors.New("503"),
	}
	cleanup := &upload.CleanupFailedError{
		UploadID: "up-3-cleanup",
		Key:      "attachments/u/k/original/file3.bin",
		Cause:    errors.New("abort 500"),
	}
	joined := errors.Join(state, cleanup)
	detail := classifyCommandError(joined)
	if detail.Code != "complete_state_unknown" {
		t.Fatalf("code = %q, want complete_state_unknown", detail.Code)
	}
	// The state's handles take priority; cleanup fills only missing fields.
	if detail.Recovery.UploadID != "up-3" {
		t.Errorf("UploadID = %q, want up-3", detail.Recovery.UploadID)
	}
}

func TestPrintUploadRecovery_HumanMode_IncludesFullManifest(t *testing.T) {
	state := &upload.UnknownStateError{
		FileURL:  "https://example.com/attachments/u/k/file.bin",
		UploadID: "up-1",
		Key:      "attachments/u/k/original/file.bin",
		CompletedParts: []upload.CompletedPart{
			{PartNumber: 1, ETag: "etag-alpha"},
			{PartNumber: 2, ETag: "etag-beta"},
		},
		Cause: errors.New("503"),
	}
	var buf bytes.Buffer
	style := output.NewStylerForWriter(&buf, true)
	printUploadRecovery(&buf, style, state)
	got := buf.String()
	for _, want := range []string{
		"Recovery:",
		"file_url:",
		"upload_id:",
		"key:",
		"completed_parts: 2",
		"part_number=1 etag=etag-alpha",
		"part_number=2 etag=etag-beta",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestPrintUploadRecovery_CleanupOnly_PrintsOrphanHandles(t *testing.T) {
	// A cleanup failure without an UnknownStateError still carries upload_id
	// and key; the human recovery block must print them, otherwise the
	// "reclaim with gumroad files abort" hint has nothing to reference.
	cleanup := &upload.CleanupFailedError{
		UploadID: "up-orphan",
		Key:      "attachments/u/k/original/orphan.bin",
		Cause:    errors.New("abort 500"),
	}
	var buf bytes.Buffer
	style := output.NewStylerForWriter(&buf, true)
	printUploadRecovery(&buf, style, cleanup)
	got := buf.String()
	for _, want := range []string{"upload_id: up-orphan", "key:       attachments/u/k/original/orphan.bin", "gumroad files abort"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestPrintUploadRecovery_APIErrorJoinedWithCleanup_PrintsOrphanHandles(t *testing.T) {
	primary := &api.APIError{StatusCode: 200, Message: "complete rejected"}
	cleanup := &upload.CleanupFailedError{
		UploadID: "up-joined",
		Key:      "attachments/u/k/original/joined.bin",
		Cause:    errors.New("abort 500"),
	}
	joined := errors.Join(primary, cleanup)

	var buf bytes.Buffer
	style := output.NewStylerForWriter(&buf, true)
	printUploadRecovery(&buf, style, joined)
	got := buf.String()
	for _, want := range []string{"upload_id: up-joined", "key:       attachments/u/k/original/joined.bin"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestClassifyCommandError_APIErrorJoinedWithCleanup_PreservesPrimary(t *testing.T) {
	primary := &api.APIError{StatusCode: 200, Message: "complete rejected"}
	cleanup := &upload.CleanupFailedError{UploadID: "up-7", Key: "attachments/u/k/original/p.bin", Cause: errors.New("abort 500")}
	joined := errors.Join(primary, cleanup)

	detail := classifyCommandError(joined)
	if detail.Type != "api_error" {
		t.Fatalf("type = %q, want api_error (primary must win over cleanup)", detail.Type)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-7" {
		t.Errorf("expected cleanup orphan handles attached, got %+v", detail.Recovery)
	}
}

func TestClassifyCommandError_APIErrorJoinedWithWrappedCleanup_PreservesPrimary(t *testing.T) {
	primary := &api.APIError{StatusCode: 403, Message: "Access denied"}
	cleanup := fmt.Errorf("abort retry exhausted: %w", &upload.CleanupFailedError{
		UploadID: "up-wrapped",
		Key:      "attachments/u/k/original/wrapped.bin",
		Cause:    errors.New("abort 500"),
	})
	joined := errors.Join(primary, cleanup)

	detail := classifyCommandError(joined)
	if detail.Type != "api_error" || detail.Code != "access_denied" {
		t.Fatalf("type/code = %q/%q, want api_error/access_denied", detail.Type, detail.Code)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-wrapped" {
		t.Errorf("expected wrapped cleanup orphan handles attached, got %+v", detail.Recovery)
	}
}

// When abort returns an HTTP error, CleanupFailedError.Unwrap exposes it as
// an *api.APIError. Classification must still report cleanup_failed, not the
// abort's api_error, so callers know there's an orphan to reclaim.
func TestClassifyCommandError_CleanupCauseIsAPIError_ClassifiesAsCleanup(t *testing.T) {
	abortErr := &api.APIError{StatusCode: 500, Message: "abort returned 500"}
	cleanup := &upload.CleanupFailedError{
		UploadID: "up-abort-500",
		Key:      "attachments/u/k/original/p.bin",
		Cause:    abortErr,
	}
	// Simulate the joinAbort shape: primary uploadErr (context canceled) + cleanup.
	joined := errors.Join(context.Canceled, cleanup)

	detail := classifyCommandError(joined)
	if detail.Type != "upload_error" || detail.Code != "cleanup_failed" {
		t.Fatalf("type/code = %q/%q, want upload_error/cleanup_failed", detail.Type, detail.Code)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-abort-500" {
		t.Errorf("recovery = %+v", detail.Recovery)
	}
}

func TestUploadIncompleteDetail_MissingFileURL_AvoidsFileURLHint(t *testing.T) {
	// Presign may omit file_url per the upload package contract; the hint
	// must not tell users to "check the returned file_url" in that case.
	state := &upload.UnknownStateError{
		UploadID: "up-no-url",
		Key:      "attachments/u/k/original/p.bin",
		Cause:    errors.New("503"),
	}
	detail := classifyCommandError(state)
	if strings.Contains(detail.Hint, "file_url") {
		t.Errorf("hint mentions file_url when it was absent: %q", detail.Hint)
	}
	if !strings.Contains(detail.Hint, "files abort") {
		t.Errorf("hint should still point to abort: %q", detail.Hint)
	}
}

func TestUploadIncompleteDetail_HintDoesNotClaimFileURLAuthoritative(t *testing.T) {
	// The presign file_url is a non-authoritative hint — the hint copy
	// must not imply checking it proves commit state.
	state := &upload.UnknownStateError{
		FileURL:  "https://example.com/attachments/u/k/file.bin",
		UploadID: "up-with-url",
		Key:      "attachments/u/k/original/p.bin",
		Cause:    errors.New("503"),
	}
	detail := classifyCommandError(state)
	if strings.Contains(detail.Hint, "check the returned file_url") {
		t.Errorf("hint must not frame presign file_url as authoritative commit check: %q", detail.Hint)
	}
	if !strings.Contains(detail.Hint, "files complete") {
		t.Errorf("hint should point to files complete: %q", detail.Hint)
	}
}

func TestClassifyCommandError_CompleteRejected_NonAuthIsFirstClassUploadError(t *testing.T) {
	// A 400 "invalid etag" rejection is an upload-recovery problem, not
	// an auth issue: surface it as a first-class upload_error/complete_rejected
	// with the full manifest attached as Recovery metadata.
	rejected := &files.CompleteRejectedError{
		FileURL:  "https://example.com/attachments/u/k/file.bin",
		UploadID: "up-r",
		Key:      "attachments/u/k/original/file.bin",
		CompletedParts: []upload.CompletedPart{
			{PartNumber: 1, ETag: "etag-1"},
		},
		Cause: &api.APIError{StatusCode: 400, Message: "invalid etag"},
	}
	detail := classifyCommandError(rejected)
	if detail.Type != "upload_error" || detail.Code != "complete_rejected" {
		t.Fatalf("type/code = %q/%q, want upload_error/complete_rejected", detail.Type, detail.Code)
	}
	if detail.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400 (preserved from underlying APIError)", detail.StatusCode)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-r" || len(detail.Recovery.CompletedParts) != 1 {
		t.Errorf("Recovery = %+v", detail.Recovery)
	}
}

func TestClassifyCommandError_CompleteRejected_AuthCausePreservesAuthClassification(t *testing.T) {
	// When the rejection is a 401/403, the caller needs to re-authenticate
	// first; surface api_error/not_authenticated so the normal auth
	// remediation path fires, while keeping the orphan handles as
	// Recovery metadata for later cleanup.
	rejected := &files.CompleteRejectedError{
		UploadID: "up-auth",
		Key:      "attachments/u/k/original/p.bin",
		CompletedParts: []upload.CompletedPart{
			{PartNumber: 1, ETag: "e1"},
		},
		Cause: &api.APIError{StatusCode: 401, Message: "not_authenticated"},
	}
	detail := classifyCommandError(rejected)
	if detail.Type != "api_error" || detail.Code != "not_authenticated" {
		t.Fatalf("type/code = %q/%q, want api_error/not_authenticated", detail.Type, detail.Code)
	}
	if detail.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", detail.StatusCode)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-auth" {
		t.Errorf("Recovery should still carry orphan handles: %+v", detail.Recovery)
	}
}

func TestClassifyCommandError_PresignExpired(t *testing.T) {
	detail := classifyCommandError(upload.ErrPresignExpired)
	if detail.Type != "upload_error" || detail.Code != "presign_expired" {
		t.Fatalf("type/code = %q/%q, want upload_error/presign_expired", detail.Type, detail.Code)
	}
	if !strings.Contains(detail.Hint, "Re-run") {
		t.Errorf("expected restart-from-scratch hint, got %q", detail.Hint)
	}
}

func TestClassifyCommandError_PresignExpiredWithCleanup_KeepsPresignCode(t *testing.T) {
	// If cleanup also failed, the presign_expired primary classification
	// must survive the post-hoc cleanup merge — internal_error would
	// upgrade to cleanup_failed, but a recognized upload code must not.
	cleanup := &upload.CleanupFailedError{UploadID: "up-p", Key: "k", Cause: errors.New("abort 500")}
	joined := errors.Join(upload.ErrPresignExpired, cleanup)
	detail := classifyCommandError(joined)
	if detail.Code != "presign_expired" {
		t.Fatalf("code = %q, want presign_expired", detail.Code)
	}
	if detail.Recovery == nil || detail.Recovery.UploadID != "up-p" {
		t.Errorf("cleanup recovery not attached: %+v", detail.Recovery)
	}
}

func TestPrintUploadRecovery_NoopForUnrelatedError(t *testing.T) {
	var buf bytes.Buffer
	style := output.NewStylerForWriter(&buf, true)
	printUploadRecovery(&buf, style, errors.New("unrelated"))
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}
