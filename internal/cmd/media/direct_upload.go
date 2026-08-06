package media

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

const (
	maxDirectUploadErrorBody      = 4 * 1024
	directUploadMaxAttempts       = 3
	directUploadInitialRetryDelay = 100 * time.Millisecond
	directUploadMaxRetryDelay     = 500 * time.Millisecond
)

var directUploadAttemptTimeout = 2 * time.Minute

type directUploadResponse struct {
	SignedID     string `json:"signed_id"`
	Key          string `json:"key"`
	DirectUpload struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"direct_upload"`
}

type directUploadStatusError struct {
	StatusCode int
	Message    string
}

type directUploadTransportError struct {
	Cause error
}

type directUploadStateUnknownError struct {
	Cause error
}

func (e *directUploadStateUnknownError) Error() string {
	return "direct upload completion is unknown: " + e.Cause.Error()
}

func (e *directUploadStateUnknownError) Unwrap() error {
	return e.Cause
}

func (e *directUploadTransportError) Error() string {
	return "direct upload failed: " + e.Cause.Error()
}

func (e *directUploadTransportError) Unwrap() error {
	return e.Cause
}

func (e *directUploadStatusError) Error() string {
	return fmt.Sprintf("direct upload failed with HTTP %d: %s", e.StatusCode, e.Message)
}

func reserveDirectUpload(client *api.Client, plan plannedMediaUpload) (directUploadResponse, error) {
	params := url.Values{}
	params.Set("purpose", "media")
	params.Set("blob[filename]", plan.Filename)
	params.Set("blob[byte_size]", strconv.FormatInt(plan.Size, 10))
	params.Set("blob[checksum]", plan.Checksum)
	params.Set("blob[content_type]", plan.ContentType)

	data, err := client.Post("/direct_uploads", params)
	if err != nil {
		return directUploadResponse{}, err
	}
	resp, err := cmdutil.DecodeJSON[directUploadResponse](data)
	if err != nil {
		return directUploadResponse{}, err
	}
	if resp.SignedID == "" {
		return directUploadResponse{}, fmt.Errorf("direct upload response did not include signed_id")
	}
	if resp.DirectUpload.URL == "" {
		return directUploadResponse{}, fmt.Errorf("direct upload response did not include upload URL")
	}
	if resp.Key == "" {
		return directUploadResponse{}, fmt.Errorf("direct upload response did not include storage key")
	}
	return resp, nil
}

// Tests in this package must not use t.Parallel while they replace this client.
var s3HTTPClientForTesting *http.Client

func putDirectUpload(opts cmdutil.Options, plan plannedMediaUpload, uploadURL string, headers map[string]string) error {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	client := directUploadHTTPClient()
	var lastErr error
	outcomeUnknown := false

	for attempt := 0; attempt < directUploadMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return directUploadResult(err, outcomeUnknown)
		}
		err := putDirectUploadAttempt(ctx, client, plan, uploadURL, headers)
		if err == nil {
			return nil
		}
		if isRetryableDirectUploadError(err) {
			outcomeUnknown = true
		}
		if err := ctx.Err(); err != nil {
			return directUploadResult(err, outcomeUnknown)
		}
		if !isRetryableDirectUploadError(err) {
			return directUploadResult(err, outcomeUnknown)
		}
		lastErr = err
		if attempt+1 < directUploadMaxAttempts {
			if err := waitForDirectUploadRetry(ctx, attempt); err != nil {
				return directUploadResult(err, outcomeUnknown)
			}
		}
	}

	return directUploadResult(fmt.Errorf("direct upload failed after %d attempts: %w", directUploadMaxAttempts, lastErr), outcomeUnknown)
}

func directUploadResult(err error, outcomeUnknown bool) error {
	if outcomeUnknown {
		return &directUploadStateUnknownError{Cause: err}
	}
	return err
}

func directUploadHTTPClient() *http.Client {
	if s3HTTPClientForTesting != nil {
		return s3HTTPClientForTesting
	}
	return &http.Client{Timeout: directUploadAttemptTimeout}
}

func putDirectUploadAttempt(ctx context.Context, client *http.Client, plan plannedMediaUpload, uploadURL string, headers map[string]string) error {
	attemptCtx, cancel := context.WithTimeout(ctx, directUploadAttemptTimeout)
	defer cancel()

	file, err := os.Open(plan.Path)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", plan.Path, err)
	}
	defer func() { _ = file.Close() }()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPut, uploadURL, file)
	if err != nil {
		return fmt.Errorf("could not create direct upload request: %w", err)
	}
	req.ContentLength = plan.Size
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", plan.ContentType)
	}
	if req.Header.Get("Content-MD5") == "" {
		req.Header.Set("Content-MD5", plan.Checksum)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &directUploadTransportError{Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDirectUploadErrorBody))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return &directUploadStatusError{StatusCode: resp.StatusCode, Message: message}
}

func isRetryableDirectUploadError(err error) bool {
	var statusErr *directUploadStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusRequestTimeout ||
			statusErr.StatusCode == http.StatusConflict ||
			statusErr.StatusCode == http.StatusTooEarly ||
			statusErr.StatusCode == http.StatusTooManyRequests ||
			statusErr.StatusCode >= 500
	}
	var transportErr *directUploadTransportError
	if !errors.As(err, &transportErr) {
		return false
	}
	if errors.Is(transportErr.Cause, context.DeadlineExceeded) {
		return true
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var certificateError x509.CertificateInvalidError
	if errors.As(transportErr.Cause, &unknownAuthority) ||
		errors.As(transportErr.Cause, &hostnameError) ||
		errors.As(transportErr.Cause, &certificateError) {
		return false
	}
	var networkErr net.Error
	return errors.As(transportErr.Cause, &networkErr)
}

func waitForDirectUploadRetry(ctx context.Context, attempt int) error {
	delay := directUploadInitialRetryDelay << attempt
	if delay > directUploadMaxRetryDelay {
		delay = directUploadMaxRetryDelay
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
