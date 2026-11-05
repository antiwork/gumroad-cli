package media

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
)

type MediaUploadStage string

const (
	MediaUploadStageDirectUpload MediaUploadStage = "direct_upload"
	MediaUploadStageCommit       MediaUploadStage = "commit"
)

const (
	mediaUploadRateLimit  = 20
	mediaUploadRateWindow = 10 * time.Minute
)

// UnknownMediaUploadError means the CLI cannot prove whether an upload stage
// completed. The signed blob ID and storage key identify the reserved blob.
type UnknownMediaUploadError struct {
	Cause        error
	Stage        MediaUploadStage
	SignedBlobID string
	BlobKey      string
	Name         string
	Filename     string
	FileSize     int64
}

type CommittedMediaOutputError struct {
	Cause    error
	MediaID  string
	MediaURL string
}

func (e *CommittedMediaOutputError) Error() string {
	return fmt.Sprintf("media upload completed as %s, but output failed: %s", e.MediaID, e.Cause)
}

func (e *CommittedMediaOutputError) Unwrap() error {
	return e.Cause
}

func (e *CommittedMediaOutputError) RecoveryHint() string {
	return fmt.Sprintf("The upload completed. Do not retry it. The media ID is %s and the URL is %s.", e.MediaID, e.MediaURL)
}

func (e *UnknownMediaUploadError) Error() string {
	return fmt.Sprintf("media upload state is unknown after %s: %s", e.Stage, e.Cause)
}

func (e *UnknownMediaUploadError) Unwrap() error {
	return e.Cause
}

func (e *UnknownMediaUploadError) RecoveryHint() string {
	if e.Stage == MediaUploadStageCommit {
		return fmt.Sprintf("Run `gumroad media list --json --no-input` and find the item whose URL contains storage key %q. If it exists, the upload completed. Do not retry automatically if it is absent because the first request can still finish. Keep the signed_blob_id and key for support.", e.BlobKey)
	}
	return "Do not start a new upload. The direct upload result is unknown. Keep the signed_blob_id and key for support."
}

func mediaDirectUploadError(err error, reservation directUploadResponse, plan plannedMediaUpload, name string) error {
	var unknown *directUploadStateUnknownError
	if !errors.As(err, &unknown) {
		return err
	}
	return newUnknownMediaUploadError(err, MediaUploadStageDirectUpload, reservation, plan, name)
}

func mediaCommitError(err error, reservation directUploadResponse, plan plannedMediaUpload, name string) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		return mediaRateLimitError(apiErr)
	}
	if errors.As(err, &apiErr) && isDefinitiveMediaMutationRejection(apiErr.StatusCode) {
		return err
	}
	return newUnknownMediaUploadError(err, MediaUploadStageCommit, reservation, plan, name)
}

func mediaRateLimitError(apiErr *api.APIError) error {
	wait := apiErr.RetryAfter
	if wait <= 0 {
		wait = mediaUploadRateWindow
	}
	return &api.APIError{
		StatusCode: apiErr.StatusCode,
		Message:    "media upload was rate limited. Nothing was created in your media library.",
		Hint: fmt.Sprintf(
			"Wait about %s, then run the upload again. Gumroad allows %d media uploads per %s for each token and for each IP address, so tokens on the same network share one budget. To upload many images, leave at least %s between uploads.",
			humanWait(wait),
			mediaUploadRateLimit,
			humanWait(mediaUploadRateWindow),
			humanWait(mediaUploadRateWindow/mediaUploadRateLimit),
		),
		RetryAfter: wait,
	}
}

func humanWait(d time.Duration) string {
	if d >= time.Minute {
		return pluralize(int((d+time.Minute-1)/time.Minute), "minute")
	}
	return pluralize(int((d+time.Second-1)/time.Second), "second")
}

func pluralize(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func mediaCommitResponseError(err error, reservation directUploadResponse, plan plannedMediaUpload, name string) error {
	return newUnknownMediaUploadError(err, MediaUploadStageCommit, reservation, plan, name)
}

func mediaOutputError(err error, media mediaItem) error {
	return &CommittedMediaOutputError{Cause: err, MediaID: media.ID, MediaURL: media.URL}
}

func newUnknownMediaUploadError(err error, stage MediaUploadStage, reservation directUploadResponse, plan plannedMediaUpload, name string) error {
	return &UnknownMediaUploadError{
		Cause:        err,
		Stage:        stage,
		SignedBlobID: reservation.SignedID,
		BlobKey:      reservation.Key,
		Name:         name,
		Filename:     plan.Filename,
		FileSize:     plan.Size,
	}
}
