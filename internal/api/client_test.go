package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		srv.Close()
	})
	return srv
}

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		token:      "test-token",
		httpClient: srv.Client(),
		baseURL:    srv.URL,
		version:    "test",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func assertAPIError(t *testing.T, err error, wantStatusCode int, wantMessage string) {
	t.Helper()

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != wantStatusCode {
		t.Errorf("got status %d, want %d", apiErr.StatusCode, wantStatusCode)
	}
	if apiErr.Message != wantMessage {
		t.Errorf("got message %q, want %q", apiErr.Message, wantMessage)
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("tok-123", "1.0.0", false)
	if c.token != "tok-123" {
		t.Errorf("got token=%q, want tok-123", c.token)
	}
	if c.version != "1.0.0" {
		t.Errorf("got version=%q, want 1.0.0", c.version)
	}
	if c.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
	if c.debug {
		t.Error("debug should default to false")
	}
}

func TestClient_BearerHeader(t *testing.T) {
	var gotAuth string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := &Client{token: "test-token-123", httpClient: srv.Client(), baseURL: srv.URL, version: "test"}

	_, err := c.Get("/user", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token-123" {
		t.Errorf("got Authorization=%q, want %q", gotAuth, "Bearer test-token-123")
	}
}

func TestClient_UserAgent(t *testing.T) {
	var gotUA string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := &Client{token: "tok", httpClient: srv.Client(), baseURL: srv.URL, version: "1.2.3"}

	_, _ = c.Get("/user", nil)
	if gotUA != "gumroad-cli/1.2.3" {
		t.Errorf("got User-Agent=%q, want %q", gotUA, "gumroad-cli/1.2.3")
	}
}

func TestClient_HTTP4xxError(t *testing.T) {
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Not found"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Get("/products/bad", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertAPIError(t, err, 404, "Not found")
}

func TestClient_SuccessFalseError(t *testing.T) {
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Invalid resource"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Get("/anything", nil)
	if err == nil {
		t.Fatal("expected error for success=false, got nil")
	}
	assertAPIError(t, err, 200, "Invalid resource")
}

func TestClient_SuccessFalse_NoMessage(t *testing.T) {
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Get("/anything", nil)
	if err == nil {
		t.Fatal("expected error for success=false without message, got nil")
	}
	assertAPIError(t, err, 200, "API error (HTTP 200)")
}

func TestClient_PostSendsFormBody(t *testing.T) {
	var gotContentType string
	var gotBody string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotBody = r.PostForm.Get("name")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	params := make(map[string][]string)
	params["name"] = []string{"test-value"}
	_, err := c.Post("/test", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("got Content-Type=%q, want form-urlencoded", gotContentType)
	}
	if gotBody != "test-value" {
		t.Errorf("got body name=%q, want %q", gotBody, "test-value")
	}
}

func TestClient_Put(t *testing.T) {
	var gotMethod string
	var gotBody string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotBody = r.PostForm.Get("title")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	params := make(map[string][]string)
	params["title"] = []string{"updated"}
	_, err := c.Put("/resource/1", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotBody != "updated" {
		t.Errorf("got body title=%q, want updated", gotBody)
	}
}

func TestClient_PutJSON(t *testing.T) {
	var gotMethod string
	var gotContentType string
	var gotBody map[string]any
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	payload := map[string]any{
		"files": []any{},
		"name":  "updated",
	}
	_, err := c.PutJSON("/resource/1", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("got Content-Type=%q, want application/json", gotContentType)
	}
	if gotBody["name"] != "updated" {
		t.Errorf("got name=%v, want updated", gotBody["name"])
	}
	files, ok := gotBody["files"].([]any)
	if !ok || len(files) != 0 {
		t.Errorf("got files=%#v, want empty array", gotBody["files"])
	}
}

func TestClient_PostJSON(t *testing.T) {
	var gotMethod string
	var gotContentType string
	var gotBody map[string]any
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	payload := map[string]any{
		"upload_id": "up-1",
		"parts": []any{
			map[string]any{"part_number": 1, "etag": "etag-1"},
		},
	}
	_, err := c.PostJSON("/files/complete", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("got Content-Type=%q, want application/json", gotContentType)
	}
	if gotBody["upload_id"] != "up-1" {
		t.Errorf("got upload_id=%v, want up-1", gotBody["upload_id"])
	}
	parts, ok := gotBody["parts"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("got parts=%#v, want 1-element array", gotBody["parts"])
	}
}

func TestClient_Delete(t *testing.T) {
	var gotMethod string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Delete("/resource/1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("got method %q, want DELETE", gotMethod)
	}
}

func TestClient_GetAppendsQuery(t *testing.T) {
	var gotPath string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)

	params := make(map[string][]string)
	params["page"] = []string{"2"}
	_, _ = c.Get("/sales", params)

	if gotPath != "/sales?page=2" {
		t.Errorf("got path %q, want %q", gotPath, "/sales?page=2")
	}
}

func TestClient_PostNilParams(t *testing.T) {
	var gotContentType string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Post("/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "" {
		t.Errorf("expected no Content-Type for nil params, got %q", gotContentType)
	}
}

func TestClient_GetWithEmptyParamsDoesNotAddTrailingQuestionMark(t *testing.T) {
	var gotPath string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Get("/sales", url.Values{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/sales" {
		t.Errorf("got path %q, want /sales", gotPath)
	}
}

func TestClient_NetworkError(t *testing.T) {
	c := NewClient("tok", "test", false)
	c.baseURL = "http://127.0.0.1:1" // unreachable port
	_, err := c.Get("/anything", nil)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
}

func TestClient_GetRetriesTransientStatus(t *testing.T) {
	attempts := 0
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Try again"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "user": map[string]any{"email": "retry@example.com"}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	data, err := c.Get("/user", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("got %d attempts, want 2", attempts)
	}
	if len(slept) != 1 || slept[0] != time.Second {
		t.Fatalf("got retry sleeps %v, want [1s]", slept)
	}
	if !strings.Contains(string(data), "retry@example.com") {
		t.Fatalf("expected successful retry response, got %s", data)
	}
}

func TestClient_PostDoesNotRetryTransientStatus(t *testing.T) {
	attempts := 0
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Try again"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	c := newTestClient(srv)
	c.sleep = func(context.Context, time.Duration) error {
		t.Fatal("unexpected retry sleep for POST")
		return nil
	}

	_, err := c.Post("/sales/refund", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertAPIError(t, err, http.StatusServiceUnavailable, "Try again")
	if attempts != 1 {
		t.Fatalf("got %d attempts, want 1", attempts)
	}
}

func TestClient_GetRetryCanceledDuringBackoff(t *testing.T) {
	attempts := 0
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Try again"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	c := newTestClient(srv)
	c.ctx = ctx
	c.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}

	_, err := c.Get("/user", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err=%v, want context canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("got %d attempts, want 1", attempts)
	}
}

func TestClient_GetRetriesBodyReadFailure(t *testing.T) {
	attempts := 0
	c := &Client{
		token:   "test-token",
		baseURL: "http://localhost",
		version: "test",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(errReader{err: io.ErrUnexpectedEOF}),
					}, nil
				}
				body := `{"success":true,"value":"ok"}`
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
	}
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	data, err := c.Get("/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("got %d attempts, want 2", attempts)
	}
	if len(slept) != 1 {
		t.Fatalf("got %d sleeps, want 1", len(slept))
	}
	if !strings.Contains(string(data), "ok") {
		t.Fatalf("unexpected response: %s", data)
	}
}

func TestClient_PostDoesNotRetryBodyReadFailure(t *testing.T) {
	attempts := 0
	c := &Client{
		token:   "test-token",
		baseURL: "http://localhost",
		version: "test",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(errReader{err: io.ErrUnexpectedEOF}),
				}, nil
			}),
		},
	}
	c.sleep = func(context.Context, time.Duration) error {
		t.Fatal("unexpected retry sleep for POST")
		return nil
	}

	_, err := c.Post("/test", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("got %d attempts, want 1", attempts)
	}
}

func TestClient_GetDoesNotRetryOversizedBody(t *testing.T) {
	attempts := 0
	oversized := strings.Repeat("x", maxResponseBodySize+1)
	c := &Client{
		token:   "test-token",
		baseURL: "http://localhost",
		version: "test",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(oversized)),
				}, nil
			}),
		},
	}
	c.sleep = func(context.Context, time.Duration) error {
		t.Fatal("unexpected retry sleep for oversized body")
		return nil
	}

	_, err := c.Get("/test", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("got %d attempts, want 1", attempts)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	delay, ok := parseRetryAfter("2")
	if !ok {
		t.Fatal("expected seconds Retry-After to parse")
	}
	if delay != 2*time.Second {
		t.Fatalf("got %s, want 2s", delay)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	retryAt := time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
	delay, ok := parseRetryAfter(retryAt)
	if !ok {
		t.Fatal("expected HTTP date Retry-After to parse")
	}
	if delay <= 0 {
		t.Fatalf("expected positive delay, got %s", delay)
	}
}

func TestParseRetryAfter_InvalidValues(t *testing.T) {
	for _, input := range []string{"", "0", "garbage", time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat)} {
		if delay, ok := parseRetryAfter(input); ok {
			t.Fatalf("expected %q to be rejected, got delay %s", input, delay)
		}
	}
}

func TestClient_DebugOffProducesNoLogs(t *testing.T) {
	var logs bytes.Buffer
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)
	c.debugWriter = &logs

	if _, err := c.Get("/user", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no debug logs, got %q", logs.String())
	}
}

func TestClient_DebugSuccessLogsMetadata(t *testing.T) {
	var logs bytes.Buffer
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "user": map[string]any{"email": "debug@example.com"}}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)
	c.debug = true
	c.debugWriter = &logs

	if _, err := c.Get("/user", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := logs.String()
	for _, want := range []string{
		"DEBUG request method=GET",
		"url=" + srv.URL + "/user",
		"status=200",
		"bytes=",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug log missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "test-token") {
		t.Fatalf("debug log should not contain bearer token: %q", out)
	}
}

func TestClient_DebugRedactsQueryValues(t *testing.T) {
	var logs bytes.Buffer
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "sales": []any{}}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)
	c.debug = true
	c.debugWriter = &logs

	params := url.Values{
		"email":    []string{"buyer@example.com"},
		"page_key": []string{"cursor123"},
	}
	if _, err := c.Get("/sales", params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := logs.String()
	if strings.Contains(out, "buyer@example.com") || strings.Contains(out, "cursor123") {
		t.Fatalf("debug log should redact query values: %q", out)
	}
	for _, want := range []string{"email=REDACTED", "page_key=REDACTED"} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug log missing redacted query marker %q in %q", want, out)
		}
	}
}

func TestClient_DebugAPIErrorLogsNormalizedError(t *testing.T) {
	var logs bytes.Buffer
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Invalid resource"}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)
	c.debug = true
	c.debugWriter = &logs

	_, err := c.Get("/products/bad", nil)
	if err == nil {
		t.Fatal("expected error for success=false")
	}

	out := logs.String()
	if !strings.Contains(out, `api_error="Invalid resource"`) {
		t.Fatalf("debug log missing normalized API error in %q", out)
	}
	if !strings.Contains(out, "status=200") {
		t.Fatalf("debug log missing status in %q", out)
	}
}

func TestClient_HTTP401Error(t *testing.T) {
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Get("/user", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("got status %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Message != "Not authenticated." {
		t.Errorf("got message %q", apiErr.Message)
	}
	if apiErr.Hint != HintRunAuthLogin {
		t.Errorf("got hint %q", apiErr.Hint)
	}
}

func TestClient_HTTP500_NoJSON(t *testing.T) {
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		if _, err := w.Write([]byte("Internal Server Error")); err != nil {
			t.Fatalf("write response: %v", err)
		}
	})
	c := newTestClient(srv)

	_, err := c.Get("/anything", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("got status %d, want 500", apiErr.StatusCode)
	}
	if apiErr.Message != "API error (HTTP 500)" {
		t.Errorf("got message %q", apiErr.Message)
	}
}

func TestClient_DeleteWithParams(t *testing.T) {
	var gotContentType string
	srv := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	c := newTestClient(srv)

	params := make(map[string][]string)
	params["reason"] = []string{"test"}
	_, err := c.Delete("/resource/1", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("got Content-Type=%q, want form-urlencoded", gotContentType)
	}
}

func TestNewClient_UsesEnvBaseURL(t *testing.T) {
	t.Setenv("GUMROAD_API_BASE_URL", "http://custom.local")

	c := NewClient("tok", "test", false)
	if c.baseURL != "http://custom.local" {
		t.Errorf("got baseURL=%q, want http://custom.local", c.baseURL)
	}
}

func TestDefaultBaseURL_WithEnv(t *testing.T) {
	t.Setenv("GUMROAD_API_BASE_URL", "http://test.local")
	got := defaultBaseURL()
	if got != "http://test.local" {
		t.Errorf("got %q, want http://test.local", got)
	}
}

func TestDefaultBaseURL_Default(t *testing.T) {
	t.Setenv("GUMROAD_API_BASE_URL", "")
	got := defaultBaseURL()
	if got != "https://api.gumroad.com/v2" {
		t.Errorf("got %q, want https://api.gumroad.com/v2", got)
	}
}

func TestNewClientWithContextUsesRequestContext(t *testing.T) {
	type traceKey string

	ctx := context.WithValue(context.Background(), traceKey("trace"), "abc123")
	c := NewClientWithContext(ctx, "tok", "test", false)
	c.baseURL = "http://example.test"
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Context().Value(traceKey("trace")); got != "abc123" {
				t.Fatalf("got context value %v, want abc123", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
			}, nil
		}),
	}

	if _, err := c.Get("/user", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ResponseTooLarge(t *testing.T) {
	c := NewClient("tok", "test", false)
	c.baseURL = "http://example.test"
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'a'}, maxResponseBodySize+1))),
			}, nil
		}),
	}

	_, err := c.Get("/user", nil)
	if err == nil {
		t.Fatal("expected oversized response error")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_SetDebugWriter(t *testing.T) {
	var logs bytes.Buffer
	c := &Client{}
	c.SetDebugWriter(&logs)
	if c.debugWriter != &logs {
		t.Fatalf("got debugWriter=%v, want buffer", c.debugWriter)
	}

	var nilClient *Client
	nilClient.SetDebugWriter(&logs)
}

func TestClient_RequestContext_NormalizesNilState(t *testing.T) {
	var nilClient *Client
	if nilClient.requestContext() == nil {
		t.Fatal("nil client should fall back to background context")
	}

	c := &Client{}
	if c.requestContext() == nil {
		t.Fatal("nil client context should fall back to background context")
	}
}

func TestSleepContext_ZeroDelayReturnsImmediately(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSleepContext_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := sleepContext(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("got err=%v, want context canceled", err)
	}
}

func TestReadResponseBody_TooLarge(t *testing.T) {
	_, err := readResponseBody(bytes.NewReader(bytes.Repeat([]byte{'a'}, maxResponseBodySize+1)))
	if err == nil {
		t.Fatal("expected oversized body error")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadResponseBody_ReadError(t *testing.T) {
	want := errors.New("read failed")
	_, err := readResponseBody(errReader{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("got err=%v, want %v", err, want)
	}
}

func TestRedactQueryValues_PreservesKeysAndCardinality(t *testing.T) {
	values := url.Values{
		"email":    []string{"buyer@example.com"},
		"page_key": []string{"page-1", "page-2"},
		"empty":    nil,
	}

	redacted := redactQueryValues(values)
	if got := redacted.Get("email"); got != "REDACTED" {
		t.Fatalf("got email=%q, want REDACTED", got)
	}
	if got := redacted["page_key"]; len(got) != 2 || got[0] != "REDACTED" || got[1] != "REDACTED" {
		t.Fatalf("got page_key=%v, want redacted values", got)
	}
	if current, ok := redacted["empty"]; !ok || current != nil {
		t.Fatalf("expected empty key to be preserved, got %v ok=%v", current, ok)
	}
	if got := values.Get("email"); got != "buyer@example.com" {
		t.Fatalf("expected original values to stay intact, got %q", got)
	}
}

func TestRetryDelay_FallbackBackoffAndCap(t *testing.T) {
	if got := retryDelay(0, "garbage"); got != initialRetryBackoff {
		t.Fatalf("got %s, want %s", got, initialRetryBackoff)
	}
	if got := retryDelay(10, ""); got != maxRetryBackoff {
		t.Fatalf("got %s, want %s", got, maxRetryBackoff)
	}
}
