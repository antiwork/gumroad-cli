package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	maxRetries          = 2
	initialRetryBackoff = 200 * time.Millisecond
	maxRetryBackoff     = 2 * time.Second
	maxResponseBodySize = 10 * 1024 * 1024
	defaultAPIBaseURL   = "https://api.gumroad.com/v2"
)

func defaultBaseURL() string {
	if v := os.Getenv("GUMROAD_API_BASE_URL"); v != "" {
		return v
	}
	return defaultAPIBaseURL
}

type Client struct {
	ctx         context.Context
	token       string
	httpClient  *http.Client
	baseURL     string
	version     string
	debug       bool
	debugWriter io.Writer
	sleep       func(context.Context, time.Duration) error
}

func NewClient(token, version string, debug bool) *Client {
	return NewClientWithContext(context.Background(), token, version, debug)
}

func NewClientWithContext(ctx context.Context, token, version string, debug bool) *Client {
	ctx = normalizeContext(ctx)

	return &Client{
		ctx:   ctx,
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:     defaultBaseURL(),
		version:     version,
		debug:       debug,
		debugWriter: os.Stderr,
		sleep:       sleepContext,
	}
}

func (c *Client) SetDebugWriter(w io.Writer) {
	if c == nil {
		return
	}
	c.debugWriter = w
}

func (c *Client) Get(path string, params url.Values) (json.RawMessage, error) {
	return c.do("GET", path, params)
}

func (c *Client) Post(path string, params url.Values) (json.RawMessage, error) {
	return c.do("POST", path, params)
}

func (c *Client) Put(path string, params url.Values) (json.RawMessage, error) {
	return c.do("PUT", path, params)
}

func (c *Client) Delete(path string, params url.Values) (json.RawMessage, error) {
	return c.do("DELETE", path, params)
}

func (c *Client) debugf(format string, args ...any) {
	if !c.debug {
		return
	}

	w := c.debugWriter
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "DEBUG "+format+"\n", args...)
}

func (c *Client) do(method, path string, params url.Values) (json.RawMessage, error) {
	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = defaultBaseURL()
	}

	requestURL := baseURL + path
	logURL := requestURL
	encodedParams := ""
	if params != nil {
		encodedParams = params.Encode()
		if method == "GET" && encodedParams != "" {
			requestURL += "?" + encodedParams
			logURL += "?" + redactQueryValues(params).Encode()
		}
	}

	logResponse := func(statusCode int, size int, apiErr error, started time.Time) {
		duration := time.Since(started).Round(time.Millisecond)
		if apiErr != nil {
			c.debugf("response method=%s url=%s status=%d dur=%s bytes=%d api_error=%q", method, logURL, statusCode, duration, size, apiErr.Error())
			return
		}
		c.debugf("response method=%s url=%s status=%d dur=%s bytes=%d", method, logURL, statusCode, duration, size)
	}

	for attempt := 0; ; attempt++ {
		start := time.Now()

		var body io.Reader
		if method != "GET" && encodedParams != "" {
			body = strings.NewReader(encodedParams)
		}

		req, err := http.NewRequestWithContext(c.requestContext(), method, requestURL, body)
		if err != nil {
			c.debugf("request method=%s url=%s phase=build err=%q", method, logURL, err)
			return nil, fmt.Errorf("could not create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("User-Agent", "gumroad/"+c.version)
		if body != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		c.debugf("request method=%s url=%s attempt=%d", method, logURL, attempt+1)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if shouldRetry(method, attempt, 0) {
				delay := retryDelay(attempt, "")
				c.debugf("retry method=%s url=%s attempt=%d wait=%s reason=%q", method, logURL, attempt+1, delay, err)
				if err := c.snooze(delay); err != nil {
					return nil, fmt.Errorf("request failed: %w", err)
				}
				continue
			}
			c.debugf("response method=%s url=%s phase=request dur=%s err=%q", method, logURL, time.Since(start).Round(time.Millisecond), err)
			return nil, fmt.Errorf("request failed: %w", err)
		}

		data, readErr := readResponseBody(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if isRetryableReadError(readErr) && shouldRetry(method, attempt, 0) {
				delay := retryDelay(attempt, "")
				c.debugf("retry method=%s url=%s attempt=%d wait=%s reason=%q", method, logURL, attempt+1, delay, readErr)
				if err := c.snooze(delay); err != nil {
					return nil, fmt.Errorf("request failed: %w", err)
				}
				continue
			}
			c.debugf("response method=%s url=%s status=%d phase=read dur=%s err=%q", method, logURL, resp.StatusCode, time.Since(start).Round(time.Millisecond), readErr)
			return nil, fmt.Errorf("could not read response: %w", readErr)
		}

		if shouldRetry(method, attempt, resp.StatusCode) {
			delay := retryDelay(attempt, resp.Header.Get("Retry-After"))
			c.debugf("retry method=%s url=%s attempt=%d wait=%s reason=%q", method, logURL, attempt+1, delay, resp.Status)
			if err := c.snooze(delay); err != nil {
				return nil, fmt.Errorf("request failed: %w", err)
			}
			continue
		}

		if resp.StatusCode >= 400 {
			err := parseAPIError(resp.StatusCode, data)
			logResponse(resp.StatusCode, len(data), err, start)
			return nil, err
		}

		// Gumroad returns 200 with { "success": false } for many error cases
		var envelope struct {
			Success *bool  `json:"success"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(data, &envelope); err == nil {
			if envelope.Success != nil && !*envelope.Success {
				msg, hint := rewriteError(resp.StatusCode, envelope.Message)
				err := &APIError{StatusCode: resp.StatusCode, Message: msg, Hint: hint}
				logResponse(resp.StatusCode, len(data), err, start)
				return nil, err
			}
		}

		logResponse(resp.StatusCode, len(data), nil, start)
		return data, nil
	}
}

func (c *Client) requestContext() context.Context {
	if c == nil {
		return context.Background()
	}
	return normalizeContext(c.ctx)
}

func readResponseBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxResponseBodySize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxResponseBodySize {
		return nil, fmt.Errorf("response too large (limit %d bytes)", maxResponseBodySize)
	}
	return data, nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func (c *Client) snooze(delay time.Duration) error {
	if c.sleep != nil {
		return c.sleep(c.requestContext(), delay)
	}
	return sleepContext(c.requestContext(), delay)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isRetryableReadError(err error) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// Check for connection reset. On Unix this is syscall.ECONNRESET (errno 54).
	// On Windows the real error is WSAECONNRESET (errno 10054), which has a
	// different value than the POSIX compatibility alias, so we check both.
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == 10054 {
		return true
	}
	return false
}

func shouldRetry(method string, attempt int, statusCode int) bool {
	if method != http.MethodGet || attempt >= maxRetries {
		return false
	}

	switch statusCode {
	case 0, http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if delay, ok := parseRetryAfter(retryAfter); ok {
		return delay
	}

	backoff := float64(initialRetryBackoff) * math.Pow(2, float64(attempt))
	if backoff > float64(maxRetryBackoff) {
		backoff = float64(maxRetryBackoff)
	}
	return time.Duration(backoff)
}

func parseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}

	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(when)
	if delay <= 0 {
		return 0, false
	}
	return delay, true
}

func redactQueryValues(values url.Values) url.Values {
	redacted := make(url.Values, len(values))
	for key, current := range values {
		if len(current) == 0 {
			redacted[key] = nil
			continue
		}

		masked := make([]string, len(current))
		for i := range current {
			masked[i] = "REDACTED"
		}
		redacted[key] = masked
	}
	return redacted
}
