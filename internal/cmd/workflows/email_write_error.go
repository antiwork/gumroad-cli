package workflows

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/api"
)

type EmailWriteAction string

const (
	EmailWriteActionAdd    EmailWriteAction = "add"
	EmailWriteActionUpdate EmailWriteAction = "update"
)

type UnknownEmailWriteError struct {
	Cause      error
	Action     EmailWriteAction
	WorkflowID string
	EmailID    string
}

func (e *UnknownEmailWriteError) Error() string {
	return fmt.Sprintf("workflow email %s state is unknown for workflow %s: %s", e.Action, e.WorkflowID, e.Cause)
}

func (e *UnknownEmailWriteError) Unwrap() error {
	return e.Cause
}

func (e *UnknownEmailWriteError) RecoveryHint() string {
	command := fmt.Sprintf("gumroad workflows view %s --json --no-input", e.WorkflowID)
	if e.EmailID != "" {
		return fmt.Sprintf("Run `%s` and inspect email ID %s for the requested values. Do not retry automatically because the first request can still finish.", command, e.EmailID)
	}
	return fmt.Sprintf("Run `%s` and inspect email IDs and timestamps. A matching step can predate this request, so it does not prove completion. Do not retry automatically. Contact support if the state remains unclear.", command)
}

type CompletedEmailWriteOutputError struct {
	Cause      error
	Action     EmailWriteAction
	WorkflowID string
	EmailID    string
}

func (e *CompletedEmailWriteOutputError) Error() string {
	return fmt.Sprintf("workflow email %s completed as %s, but output failed: %s", e.Action, e.EmailID, e.Cause)
}

func (e *CompletedEmailWriteOutputError) Unwrap() error {
	return e.Cause
}

func (e *CompletedEmailWriteOutputError) RecoveryHint() string {
	return fmt.Sprintf("The write completed. Do not retry it. Run `gumroad workflows view %s --json --no-input` and inspect email ID %s.", e.WorkflowID, e.EmailID)
}

func emailWriteRequestError(err error, action EmailWriteAction, workflowID, emailID string) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && isDefinitiveEmailWriteRejection(apiErr.StatusCode) {
		return err
	}
	return newUnknownEmailWriteError(err, action, workflowID, emailID)
}

func newUnknownEmailWriteError(err error, action EmailWriteAction, workflowID, emailID string) error {
	return &UnknownEmailWriteError{
		Cause:      err,
		Action:     action,
		WorkflowID: workflowID,
		EmailID:    emailID,
	}
}

func emailWriteOutputError(err error, action EmailWriteAction, workflowID, emailID string) error {
	return &CompletedEmailWriteOutputError{
		Cause:      err,
		Action:     action,
		WorkflowID: workflowID,
		EmailID:    emailID,
	}
}

// A transient response does not prove whether the server committed the write.
// The caller must reconcile the workflow before it retries the mutation.
func isDefinitiveEmailWriteRejection(status int) bool {
	if status >= http.StatusInternalServerError {
		return false
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	}
	return true
}
