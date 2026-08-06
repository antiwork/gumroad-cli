package media

import (
	"errors"
	"fmt"

	"github.com/antiwork/gumroad-cli/internal/api"
)

type UnknownMediaDeleteError struct {
	Cause   error
	MediaID string
}

func (e *UnknownMediaDeleteError) Error() string {
	return fmt.Sprintf("media deletion state is unknown for %s: %s", e.MediaID, e.Cause)
}

func (e *UnknownMediaDeleteError) Unwrap() error {
	return e.Cause
}

func (e *UnknownMediaDeleteError) RecoveryHint() string {
	return fmt.Sprintf("Run `gumroad media list --json --no-input` and look for media ID %s. If it is absent, the deletion completed. Do not retry automatically if it is present because the first request can still finish.", e.MediaID)
}

type CompletedMediaDeleteOutputError struct {
	Cause   error
	MediaID string
}

func (e *CompletedMediaDeleteOutputError) Error() string {
	return fmt.Sprintf("media file %s was deleted, but output failed: %s", e.MediaID, e.Cause)
}

func (e *CompletedMediaDeleteOutputError) Unwrap() error {
	return e.Cause
}

func (e *CompletedMediaDeleteOutputError) RecoveryHint() string {
	return fmt.Sprintf("The deletion completed. Do not retry it. The deleted media ID is %s.", e.MediaID)
}

func mediaDeleteOutputError(err error, mediaID string) error {
	return &CompletedMediaDeleteOutputError{Cause: err, MediaID: mediaID}
}

func mediaDeleteRequestError(err error, mediaID string) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
		return err
	}
	return &UnknownMediaDeleteError{Cause: err, MediaID: mediaID}
}
