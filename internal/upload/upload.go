// Package upload orchestrates Gumroad's S3 multipart upload flow:
// presign → parallel part PUTs → complete, with best-effort abort on failure.
package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
)

// PartSize is the fixed multipart part size the Gumroad API expects.
const PartSize int64 = 100 * 1024 * 1024

// activePartSize is the part size used by Describe and Upload. It is equal
// to PartSize in production; tests override it to avoid 100 MB+ fixtures.
var activePartSize int64 = PartSize

// allowInsecureUploadDestination is a test-only escape hatch: tests use
// httptest.NewServer (http://127.0.0.1:PORT), which would otherwise fail
// validatePresignParts's https check. Production code must leave this false.
var allowInsecureUploadDestination bool

// MaxFileSize is the server-side upload cap.
const MaxFileSize int64 = 20 * 1024 * 1024 * 1024

// DefaultConcurrency is the default number of part uploads in flight.
const DefaultConcurrency = 4

// DefaultPartTimeout bounds each part PUT attempt so a stalled S3 peer
// cannot hang the upload indefinitely. 60 minutes sets a lower throughput
// bound of roughly 230 kbps on a 100 MB part, which covers healthy slow
// links (mobile data, congested Wi-Fi) with margin. Uploads slower than
// that should set Options.PartTimeout explicitly.
const DefaultPartTimeout = 60 * time.Minute

const (
	maxPartRetries = 2
	maxS3ErrorBody = 4 * 1024

	abortTimeout = 10 * time.Second

	// errorBodyReadTimeout bounds how long we spend reading an S3 error
	// response body for classification. A slow/hostile peer that drips
	// bytes would otherwise keep the worker (and the upload's cleanup
	// path) waiting until the per-part timeout.
	errorBodyReadTimeout = 500 * time.Millisecond

	s3ExpiredMarker = "Request has expired"
)

// Backoff durations are vars so tests can shrink them; production callers
// should treat them as constants.
var (
	initialPartRetryBackoff = 200 * time.Millisecond
	maxPartRetryBackoff     = 2 * time.Second
)

// ErrPresignExpired is returned when S3 rejects a part PUT because the
// presigned URL has expired. The caller must retry the whole upload; the
// orchestrator does not refresh URLs mid-flight.
var ErrPresignExpired = errors.New("presigned URL expired; restart the upload")

// ErrCompleteStateUnknown is the sentinel matched by errors.Is on any error
// returned from Upload where /files/complete did not produce a definitive
// response (5xx, transient 4xx, transport, parse). Callers MUST verify
// server-side state before retrying — blind retry risks duplicate files.
// The concrete error implements *UnknownStateError and carries the handles
// needed for that verification.
var ErrCompleteStateUnknown = errors.New("upload state unknown; /files/complete did not return a definitive response")

// UnknownStateError is returned from Upload when the complete phase failed
// ambiguously. It carries the identifiers a caller needs to reconcile: the
// canonical file_url (to check whether the server committed — empty if the
// presign response did not include it), the upload_id/key (to abort the
// orphan later if it did not), and the completed part manifest so the
// caller can retry /files/complete without re-uploading the file.
type UnknownStateError struct {
	FileURL        string
	UploadID       string
	Key            string
	CompletedParts []CompletedPart
	Cause          error
}

// CompletedPart is one entry of the manifest passed to /files/complete.
// Callers use these to retry finalize after an ambiguous failure.
type CompletedPart struct {
	PartNumber int
	ETag       string
}

func (e *UnknownStateError) Error() string {
	return fmt.Sprintf("upload state unknown: %s", e.Cause)
}

func (e *UnknownStateError) Unwrap() error {
	return e.Cause
}

func (e *UnknownStateError) Is(target error) bool {
	return target == ErrCompleteStateUnknown
}

// CleanupFailedError wraps a /files/abort failure together with the handles
// needed to retry cleanup (e.g. from a separate job). It is always returned
// joined with the original upload error via errors.Join, so callers keep
// access to both: errors.Is on the upload cause, errors.As on this type for
// the orphan identifiers.
type CleanupFailedError struct {
	UploadID string
	Key      string
	Cause    error
}

func (e *CleanupFailedError) Error() string {
	return fmt.Sprintf("multipart cleanup failed (upload_id=%s key=%s): %s", e.UploadID, e.Key, e.Cause)
}

func (e *CleanupFailedError) Unwrap() error {
	return e.Cause
}

// Plan is the preflight view of a planned upload. It is returned by Describe
// and is suitable for --dry-run rendering; it makes no network calls.
type Plan struct {
	Path      string
	Filename  string
	Size      int64
	PartSize  int64
	PartCount int
}

// Options configures Describe and Upload.
type Options struct {
	// Filename overrides the display filename sent to presign. Defaults to
	// filepath.Base(path) when empty.
	Filename string

	// HTTPClient is used only for S3 part PUTs. Defaults to a client with no
	// overall timeout (each PUT's lifetime is bounded by context and the
	// presigned URL's 15-minute expiry). Tests inject this to redirect S3
	// traffic at an in-process httptest.Server.
	HTTPClient *http.Client

	// Concurrency bounds the number of part PUTs in flight. Defaults to
	// DefaultConcurrency. Values <= 0 use the default.
	Concurrency int

	// Progress, when non-nil, is called with the cumulative number of bytes
	// uploaded so far. Invocations are serialized and monotonic — callbacks
	// never run concurrently and the value never goes backwards. A slow or
	// blocked callback does NOT stall part uploads; it also does not keep
	// Upload from returning. The last callback may therefore run after
	// Upload has already returned the file URL.
	Progress func(bytesUploaded int64)

	// PartTimeout bounds each part PUT attempt (including retries). Zero
	// means DefaultPartTimeout. Set to a negative value to disable; negative
	// timeouts let a stalled S3 peer hang the upload indefinitely.
	PartTimeout time.Duration
}

// Describe performs local-only preflight: it stats the file, validates
// size bounds, and computes the part count. It never touches the network.
func Describe(path string, opts Options) (Plan, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Plan{}, fmt.Errorf("could not stat file: %w", err)
	}
	return planFromStat(path, info, opts)
}

// planFromStat builds a Plan from already-stat'd file info. Shared between
// Describe (public preflight, stats the path) and Upload (post-open, stats
// the open file descriptor so the upload pins one stable snapshot).
func planFromStat(path string, info os.FileInfo, opts Options) (Plan, error) {
	if info.IsDir() {
		return Plan{}, fmt.Errorf("%s is a directory", path)
	}
	if !info.Mode().IsRegular() {
		return Plan{}, fmt.Errorf("%s is not a regular file", path)
	}
	size := info.Size()
	if size == 0 {
		return Plan{}, fmt.Errorf("%s is empty", path)
	}
	if size > MaxFileSize {
		return Plan{}, fmt.Errorf("file size %d bytes exceeds maximum of %d bytes (20 GB)", size, MaxFileSize)
	}

	filename := strings.TrimSpace(opts.Filename)
	if filename == "" {
		filename = filepath.Base(path)
	}

	ps := activePartSize
	partCount := int((size + ps - 1) / ps)
	return Plan{
		Path:      path,
		Filename:  filename,
		Size:      size,
		PartSize:  ps,
		PartCount: partCount,
	}, nil
}

// Upload runs the full multipart flow and returns the canonical file_url.
// Every HTTP call — presign, part PUTs, complete, abort — is bounded by ctx.
// The *api.Client supplies auth and base URL only; its baked-in context is
// ignored so upload cancellation actually bounds all in-flight work.
func Upload(ctx context.Context, client *api.Client, path string, opts Options) (string, error) {
	// Stat first: os.Open on a FIFO/device would block the process until a
	// writer appears (Unix semantics, not bound by ctx). Checking the mode
	// up front turns that hang into a clear error. A TOCTOU race where the
	// path is swapped between Stat and Open is caught by the post-open
	// f.Stat() check below.
	preInfo, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("could not stat file: %w", err)
	}
	if !preInfo.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}

	// Pin the upload to a single FD so presign metadata and part PUTs read
	// the same bytes even if the path is mutated concurrently.
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("could not open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("could not stat file: %w", err)
	}
	plan, err := planFromStat(path, info, opts)
	if err != nil {
		return "", err
	}

	presignResp, err := presign(ctx, client, plan.Filename, plan.Size)
	if err != nil {
		// A partially-parsed response may still carry the handles needed to
		// abort an orphaned multipart upload; if so, clean up.
		if presignResp.UploadID != "" && presignResp.Key != "" {
			return "", joinAbort(err, presignResp.UploadID, presignResp.Key, safeAbort(client, presignResp.UploadID, presignResp.Key))
		}
		return "", err
	}

	if err := validatePresignParts(presignResp.Parts, plan.PartCount); err != nil {
		return "", joinAbort(err, presignResp.UploadID, presignResp.Key, safeAbort(client, presignResp.UploadID, presignResp.Key))
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = sharedS3Client
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	partTimeout := opts.PartTimeout
	if partTimeout == 0 {
		partTimeout = DefaultPartTimeout
	}

	etags, err := uploadAllParts(ctx, httpClient, f, presignResp.Parts, plan.PartSize, plan.Size, concurrency, partTimeout, opts.Progress)
	if err != nil {
		return "", joinAbort(err, presignResp.UploadID, presignResp.Key, safeAbort(client, presignResp.UploadID, presignResp.Key))
	}

	// If ctx is already canceled before we even attempt the finalize, the
	// request hasn't left the client — no server state, abort is safe.
	// Once PostWithContext runs, any transport/write failure could still
	// have delivered bytes to the server, so we can't claim that path is
	// also definitive.
	if err := ctx.Err(); err != nil {
		return "", joinAbort(err, presignResp.UploadID, presignResp.Key, safeAbort(client, presignResp.UploadID, presignResp.Key))
	}

	fileURL, err := completeUpload(ctx, client, presignResp.UploadID, presignResp.Key, presignResp.Parts, etags)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && isDefinitiveCompleteRejection(apiErr.StatusCode) {
			return "", joinAbort(err, presignResp.UploadID, presignResp.Key, safeAbort(client, presignResp.UploadID, presignResp.Key))
		}
		// Ambiguous (5xx, 408, 409, 429, transport/write/read failures):
		// the server may have committed. Skip abort so we don't race the
		// commit or tear down a successful upload.
		completed := make([]CompletedPart, len(presignResp.Parts))
		for i, p := range presignResp.Parts {
			completed[i] = CompletedPart{PartNumber: p.PartNumber, ETag: etags[i]}
		}
		return "", &UnknownStateError{
			FileURL:        presignResp.FileURL,
			UploadID:       presignResp.UploadID,
			Key:            presignResp.Key,
			CompletedParts: completed,
			Cause:          err,
		}
	}
	return fileURL, nil
}

// isDefinitiveCompleteRejection reports whether a /files/complete status code
// contractually means the request was rejected without committing — i.e.
// abort is safe. Gumroad reports many failures as HTTP 200 with
// {"success":false}, which api.Client surfaces as an APIError with
// StatusCode == 200; that is still an explicit rejection and counts as
// definitive. Transient 4xxs (408, 409, 429) and 5xx are ambiguous.
func isDefinitiveCompleteRejection(status int) bool {
	if status >= 500 {
		return false
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return false
	}
	return true
}

type presignResponse struct {
	UploadID string        `json:"upload_id"`
	Key      string        `json:"key"`
	FileURL  string        `json:"file_url"`
	Parts    []presignPart `json:"parts"`
}

type presignPart struct {
	PartNumber   int    `json:"part_number"`
	PresignedURL string `json:"presigned_url"`
}

func presign(ctx context.Context, client *api.Client, filename string, size int64) (presignResponse, error) {
	params := url.Values{}
	params.Set("filename", filename)
	params.Set("file_size", strconv.FormatInt(size, 10))

	data, err := client.PostWithContext(ctx, "/files/presign", params)
	if err != nil {
		return presignResponse{}, err
	}

	// Return whatever parsed successfully so the caller can still attempt
	// /files/abort via upload_id/key if Rails already created the multipart
	// upload but returned a malformed/incomplete response.
	var resp presignResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return resp, fmt.Errorf("could not parse presign response: %w", err)
	}
	// file_url is optional: the authoritative URL comes from /files/complete.
	// The presign copy is only used to populate UnknownStateError for ambiguous
	// complete failures, so rejecting presigns without it would be a needless
	// compatibility requirement.
	if resp.UploadID == "" || resp.Key == "" {
		return resp, errors.New("presign response missing upload_id or key")
	}
	return resp, nil
}

func completeUpload(ctx context.Context, client *api.Client, uploadID, key string, parts []presignPart, etags []string) (string, error) {
	params := url.Values{}
	params.Set("upload_id", uploadID)
	params.Set("key", key)
	for i, p := range parts {
		params.Set(fmt.Sprintf("parts[%d][part_number]", i), strconv.Itoa(p.PartNumber))
		params.Set(fmt.Sprintf("parts[%d][etag]", i), etags[i])
	}

	data, err := client.PostWithContext(ctx, "/files/complete", params)
	if err != nil {
		return "", err
	}

	var resp struct {
		FileURL string `json:"file_url"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("could not parse complete response: %w", err)
	}
	if resp.FileURL == "" {
		return "", errors.New("complete response missing file_url")
	}
	return resp.FileURL, nil
}

// safeAbort posts /files/abort under a fresh context so the cleanup attempt
// runs even when the caller's upload context is already canceled — which is
// exactly when we most want to reclaim the orphaned multipart. Bounded by
// abortTimeout so a hung backend cannot hold the caller indefinitely. It
// returns the abort error so callers can surface it (joined with the main
// error) together with the upload_id/key needed to retry cleanup later.
func safeAbort(client *api.Client, uploadID, key string) error {
	abortCtx, cancel := context.WithTimeout(context.Background(), abortTimeout)
	defer cancel()
	params := url.Values{}
	params.Set("upload_id", uploadID)
	params.Set("key", key)
	_, err := client.PostWithContext(abortCtx, "/files/abort", params)
	return err
}

// joinAbort wraps an upload failure with cleanup info when /files/abort also
// failed, so callers keep the original error semantics (errors.Is still
// matches) but can also errors.As into *CleanupFailedError to retry the
// orphan cleanup from the upload_id and key.
func joinAbort(uploadErr error, uploadID, key string, abortErr error) error {
	if abortErr == nil {
		return uploadErr
	}
	return errors.Join(uploadErr, &CleanupFailedError{
		UploadID: uploadID,
		Key:      key,
		Cause:    abortErr,
	})
}

// sharedS3Client is used by every Upload call that does not inject an
// HTTPClient. Sharing one instance avoids accumulating idle-connection
// pools and transport timers across repeated uploads in a long-lived
// process. It bounds the opaque failure modes that Go's default HTTP
// client leaves unbounded — hung TCP handshake, stalled TLS, silent peer
// — without setting http.Client.Timeout, which would cap the total PUT
// duration and kill legitimately slow uploads of a 100 MB part.
var sharedS3Client = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	},
}

// validatePresignParts guards against server-side inconsistencies (duplicate,
// missing, zero, or out-of-order part numbers) before any PUT touches S3,
// since part_number directly drives the file byte-range offset. It also
// requires each presigned URL to be HTTPS — file bytes leave this process
// over that URL, so we refuse to stream them over plain text even if the
// server is misconfigured or compromised.
func validatePresignParts(parts []presignPart, expected int) error {
	if len(parts) != expected {
		return fmt.Errorf("presign returned %d parts, expected %d", len(parts), expected)
	}
	for i, p := range parts {
		if p.PartNumber != i+1 {
			return fmt.Errorf("presign returned part_number %d at index %d, expected %d", p.PartNumber, i, i+1)
		}
		if p.PresignedURL == "" {
			return fmt.Errorf("presign returned empty URL for part %d", p.PartNumber)
		}
		if !allowInsecureUploadDestination && !strings.HasPrefix(p.PresignedURL, "https://") {
			return fmt.Errorf("presign returned non-https URL for part %d", p.PartNumber)
		}
	}
	return nil
}

func uploadAllParts(
	ctx context.Context,
	httpClient *http.Client,
	f *os.File,
	parts []presignPart,
	partSize, totalSize int64,
	concurrency int,
	partTimeout time.Duration,
	progress func(int64),
) ([]string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	etags := make([]string, len(parts))
	sem := make(chan struct{}, concurrency)

	var firstErr error
	var errMu sync.Mutex
	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	// Progress dispatch: workers enqueue cumulative-byte updates on a
	// buffered channel (cap = len(parts) so sends never block). A dedicated
	// goroutine drains the channel sequentially, which gives us both
	// serialization and monotonicity without ever holding the worker's
	// semaphore slot across a potentially-slow callback. After wg.Wait we
	// close the channel; we do not block on callback drain, so a hung
	// callback cannot keep Upload from returning.
	var progressCh chan int64
	if progress != nil {
		progressCh = make(chan int64, len(parts))
		go func() {
			for n := range progressCh {
				progress(n)
			}
		}()
	}
	var uploaded int64
	var progMu sync.Mutex
	reportProgress := func(size int64) {
		if progressCh == nil {
			return
		}
		progMu.Lock()
		uploaded += size
		progressCh <- uploaded
		progMu.Unlock()
	}

	var wg sync.WaitGroup
	for i := range parts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				setErr(ctx.Err())
				return
			}

			partNumber := parts[i].PartNumber
			offset := int64(partNumber-1) * partSize
			size := partSize
			if offset+size > totalSize {
				size = totalSize - offset
			}

			etag, err := uploadPart(ctx, httpClient, f, parts[i].PresignedURL, offset, size, partTimeout)
			// Release the slot before the potentially-slow progress callback
			// so a blocked callback cannot starve the worker pool.
			<-sem

			if err != nil {
				setErr(fmt.Errorf("upload part %d: %w", partNumber, err))
				return
			}
			etags[i] = etag
			reportProgress(size)
		}(i)
	}
	wg.Wait()
	if progressCh != nil {
		close(progressCh)
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return etags, nil
}

func uploadPart(ctx context.Context, client *http.Client, f *os.File, presignedURL string, offset, size int64, partTimeout time.Duration) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxPartRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		attemptCtx := ctx
		cancelAttempt := func() {}
		if partTimeout > 0 {
			attemptCtx, cancelAttempt = context.WithTimeout(ctx, partTimeout)
		}
		etag, err := putPart(attemptCtx, client, presignedURL, f, offset, size)
		cancelAttempt()

		if err == nil {
			return etag, nil
		}
		// Parent-ctx cancellation beats any attempt-level error: honor the
		// caller's intent to stop.
		if parentErr := ctx.Err(); parentErr != nil {
			return "", parentErr
		}
		if !isRetryablePartError(err) {
			return "", err
		}
		lastErr = err
		if attempt == maxPartRetries {
			break
		}
		if err := sleepBackoff(ctx, attempt); err != nil {
			return "", err
		}
	}
	return "", lastErr
}

func putPart(ctx context.Context, client *http.Client, presignedURL string, f *os.File, offset, size int64) (string, error) {
	body := io.NewSectionReader(f, offset, size)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignedURL, body)
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}
	req.ContentLength = size

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	// On non-2xx: read a bounded snippet for error classification, then
	// close immediately. Both the byte count (maxS3ErrorBody) and the read
	// duration (errorBodyReadTimeout) are capped so a peer that streams
	// bytes slowly — or an unbounded error body — cannot stall the worker.
	// Losing classification on a slow body just means we surface a generic
	// S3 error instead of ErrPresignExpired, which retry/abort already
	// handle.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := readBoundedBody(resp.Body, maxS3ErrorBody, errorBodyReadTimeout)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden && bytes.Contains(snippet, []byte(s3ExpiredMarker)) {
			return "", ErrPresignExpired
		}
		return "", &s3StatusError{status: resp.StatusCode, body: bytes.TrimSpace(snippet)}
	}

	// 2xx path: close immediately without draining. S3 PUT success bodies
	// are empty, so there's nothing meaningful to drain. Skipping the drain
	// forfeits connection reuse on that socket but guarantees a peer that
	// trickles bytes after the header cannot stall the worker pool.
	defer func() { _ = resp.Body.Close() }()

	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	if etag == "" {
		return "", errors.New("S3 response missing ETag header")
	}
	return etag, nil
}

// readBoundedBody reads at most maxBytes from r, giving up after timeout.
// The caller owns r and must Close it; on timeout the read goroutine will
// unblock when Close cancels the underlying Read.
func readBoundedBody(r io.Reader, maxBytes int64, timeout time.Duration) []byte {
	result := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(io.LimitReader(r, maxBytes))
		result <- b
	}()
	select {
	case b := <-result:
		return b
	case <-time.After(timeout):
		return nil
	}
}

type s3StatusError struct {
	status int
	body   []byte
}

func (e *s3StatusError) Error() string {
	if len(e.body) == 0 {
		return fmt.Sprintf("S3 returned HTTP %d", e.status)
	}
	return fmt.Sprintf("S3 returned HTTP %d: %s", e.status, e.body)
}

// isRetryablePartError classifies errors returned from a single part PUT
// attempt. The caller must have already cleared parent-context cancellation;
// context.DeadlineExceeded here therefore means the inner per-part timeout
// fired (stalled peer), which is retryable with a fresh deadline.
func isRetryablePartError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrPresignExpired) {
		return false
	}
	var s3Err *s3StatusError
	if errors.As(err, &s3Err) {
		return s3Err.status == http.StatusTooManyRequests || s3Err.status >= 500
	}
	return true
}

func sleepBackoff(ctx context.Context, attempt int) error {
	backoff := float64(initialPartRetryBackoff) * math.Pow(2, float64(attempt))
	if backoff > float64(maxPartRetryBackoff) {
		backoff = float64(maxPartRetryBackoff)
	}
	timer := time.NewTimer(time.Duration(backoff))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
