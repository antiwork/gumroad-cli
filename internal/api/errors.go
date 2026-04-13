package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

const HintRunAuthLogin = "Run: gumroad auth login — or set GUMROAD_ACCESS_TOKEN"

var (
	ErrNotAuthenticated = errors.New("not authenticated")
	ErrAccessDenied     = errors.New("access denied")
	ErrResourceNotFound = errors.New("resource not found")
)

// HintedError is implemented by errors that carry an actionable recovery suggestion.
type HintedError interface {
	error
	GetHint() string
}

type APIError struct {
	StatusCode int
	Message    string
	Hint       string
}

func (e *APIError) Error() string {
	return e.Message
}

func (e *APIError) GetHint() string {
	if e == nil {
		return ""
	}
	return e.Hint
}

func (e *APIError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}

	switch target {
	case ErrNotAuthenticated:
		return e.StatusCode == 401
	case ErrAccessDenied:
		return e.StatusCode == 403
	case ErrResourceNotFound:
		return e.StatusCode == 404
	default:
		return false
	}
}

func parseAPIError(statusCode int, body []byte) error {
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Message != "" {
		msg, hint := rewriteError(statusCode, resp.Message)
		return &APIError{StatusCode: statusCode, Message: msg, Hint: hint}
	}

	msg, hint := rewriteError(statusCode, "")
	return &APIError{StatusCode: statusCode, Message: msg, Hint: hint}
}

func rewriteError(statusCode int, msg string) (message, hint string) {
	switch statusCode {
	case 401:
		return "Not authenticated.", HintRunAuthLogin
	case 403:
		if msg != "" {
			return fmt.Sprintf("Access denied: %s", msg), "Check that your token has the required scope."
		}
		return "Access denied.", "Check that your token has the required scope."
	case 404:
		if msg != "" {
			return msg, "Check the resource ID and try again."
		}
		return "Resource not found.", "Check the resource ID and try again."
	case 429:
		if msg != "" {
			return msg, "Wait a moment and retry."
		}
		return "Rate limited.", "Wait a moment and retry."
	default:
		if msg != "" {
			return msg, ""
		}
		return fmt.Sprintf("API error (HTTP %d)", statusCode), ""
	}
}
