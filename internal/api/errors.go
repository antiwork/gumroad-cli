package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrNotAuthenticated = errors.New("not authenticated")
	ErrAccessDenied     = errors.New("access denied")
	ErrResourceNotFound = errors.New("resource not found")
)

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
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
		return &APIError{StatusCode: statusCode, Message: rewriteError(statusCode, resp.Message)}
	}

	return &APIError{StatusCode: statusCode, Message: rewriteError(statusCode, "")}
}

func rewriteError(statusCode int, msg string) string {
	switch statusCode {
	case 401:
		return "Not authenticated. Run `gumroad auth login` or set `GUMROAD_ACCESS_TOKEN`."
	case 403:
		if msg != "" {
			return fmt.Sprintf("Access denied: %s", msg)
		}
		return "Access denied. Your token may not have the required scope."
	case 404:
		if msg != "" {
			return msg
		}
		return "Resource not found."
	default:
		if msg != "" {
			return msg
		}
		return fmt.Sprintf("API error (HTTP %d)", statusCode)
	}
}
