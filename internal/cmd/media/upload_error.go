package media

import (
	"errors"
	"fmt"

	"github.com/antiwork/gumroad-cli/internal/api"
)

type MediaUploadStage string

const (
	MediaUploadStageDirectUpload MediaUploadStage = "direct_upload"
	MediaUploadStageCommit       MediaUploadStage = "commit"
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
	if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
		return err
	}
	return newUnknownMediaUploadError(err, MediaUploadStageCommit, reservation, plan, name)
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
