package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const HintRunAuthLogin = "Run: gumroad auth login. For CI/agents with an existing token, set GUMROAD_ACCESS_TOKEN or run: gumroad auth login --with-token < token.txt"

// A v2 endpoint that the token's scopes do not satisfy is refused by Doorkeeper
// with an empty 403 body, so the caller cannot tell which scope is missing. The
// endpoint's own scope refusal carries a message and is left untouched.
const HintMissingScope = "Check that your token has the required scope. A token authorized for a team store keeps only the scopes that member's role allows; run: gumroad auth login (as the store's own account for the full set)"

// A 429 is a spent budget, not a hiccup: every page of a walk is another request, and both the
// public API (per token) and gumroad.com's edge (per IP) count them, so a full history walk can
// exhaust a budget on its own. Name the budgets and the one-request alternative.
const HintRateLimited = "Rate limits count every page: the public API allows 500 requests per 10 seconds per token and gumroad.com allows 6,000 requests per hour per IP. Wait for the window to reset, narrow the range with --after/--before, or run `gumroad sales export` to fetch the whole history in one request."

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
	Code       string
	Hint       string
	RetryAfter time.Duration
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
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Error   string          `json:"error"`
		Code    json.RawMessage `json:"error_code"`
	}
	code := ""
	if err := json.Unmarshal(body, &resp); err == nil {
		if len(resp.Code) > 0 {
			_ = json.Unmarshal(resp.Code, &code)
		}
		message := resp.Message
		if message == "" {
			message = resp.Error
		}
		if message != "" {
			msg, hint := rewriteError(statusCode, message)
			return &APIError{StatusCode: statusCode, Message: msg, Code: code, Hint: hint}
		}
	}

	msg, hint := rewriteError(statusCode, "")
	return &APIError{StatusCode: statusCode, Message: msg, Code: code, Hint: hint}
}

func rewriteError(statusCode int, msg string) (message, hint string) {
	switch statusCode {
	case 401:
		return "Not authenticated.", HintRunAuthLogin
	case 403:
		if msg != "" {
			return fmt.Sprintf("Access denied: %s", msg), "Check that your token has the required scope."
		}
		return "Access denied.", HintMissingScope
	case 404:
		if msg != "" {
			return msg, "Check the resource ID and try again."
		}
		return "Resource not found.", "Check the resource ID and try again."
	case 429:
		if msg != "" {
			return msg, HintRateLimited
		}
		return "Rate limited.", HintRateLimited
	default:
		if msg != "" {
			return msg, ""
		}
		return fmt.Sprintf("API error (HTTP %d)", statusCode), ""
	}
}
