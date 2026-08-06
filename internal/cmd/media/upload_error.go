package media

import (
	"errors"
	"fmt"

	"github.com/antiwork/gumroad-cli/internal/api"
)

// UnknownMediaCommitError means the CLI cannot prove whether POST /media
// committed. Callers must reconcile the media list before they retry.
type UnknownMediaCommitError struct {
	Cause        error
	SignedBlobID string
	Name         string
	Filename     string
	FileSize     int64
}

func (e *UnknownMediaCommitError) Error() string {
	return "media upload completion is unknown: " + e.Cause.Error()
}

func (e *UnknownMediaCommitError) Unwrap() error {
	return e.Cause
}

func (e *UnknownMediaCommitError) RecoveryHint() string {
	label := e.Name
	if label == "" {
		label = e.Filename
	}
	return fmt.Sprintf("Run `gumroad media list --json --no-input` and look for a file named %q with file_size %d before you retry. If it is absent, run the upload again.", label, e.FileSize)
}

func mediaCommitError(err error, signedBlobID string, plan plannedMediaUpload, name string) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
		return err
	}
	return &UnknownMediaCommitError{
		Cause:        err,
		SignedBlobID: signedBlobID,
		Name:         name,
		Filename:     plan.Filename,
		FileSize:     plan.Size,
	}
}
