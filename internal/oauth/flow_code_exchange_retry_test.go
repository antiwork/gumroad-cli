package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type exchangeReply struct {
	status     int
	retryAfter string
	body       string
	hangUp     bool
}

func throttled(retryAfter string) exchangeReply {
	return exchangeReply{status: http.StatusTooManyRequests, retryAfter: retryAfter, body: "Retry later\n"}
}

func approved() exchangeReply {
	return exchangeReply{status: http.StatusOK, body: `{"access_token":"exchanged-token","token_type":"Bearer"}`}
}

// codeExchangeServer answers each token request with the next scripted reply
// and repeats the last one once the script runs out.
func codeExchangeServer(t *testing.T, replies []exchangeReply, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.PostForm.Get("code"); got != "auth-code" {
			t.Errorf("code = %q, want auth-code on every attempt", got)
		}
		reply := replies[min(*requests, len(replies)-1)]
		*requests++
		if reply.hangUp {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		if reply.retryAfter != "" {
			w.Header().Set("Retry-After", reply.retryAfter)
		}
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
}

// fakeClock advances only when the exchange sleeps, so tests control how much
// of the code's lifetime the retries use.
type fakeClock struct {
	now   time.Time
	waits []time.Duration
}

func (c *fakeClock) configure(cfg *FlowConfig) {
	c.now = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	cfg.now = func() time.Time { return c.now }
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		c.waits = append(c.waits, d)
		c.now = c.now.Add(d)
		return nil
	}
}

func exchangeTestConfig(srv *httptest.Server) FlowConfig {
	return FlowConfig{
		ClientID:   "test-client-id",
		TokenURL:   srv.URL + "/oauth/token",
		Timeout:    5 * time.Second,
		HTTPClient: srv.Client(),
	}
}

func TestExchangeCodeResult_RetryPolicy(t *testing.T) {
	alwaysThrottled := []exchangeReply{throttled("")}
	tests := []struct {
		name         string
		replies      []exchangeReply
		wantToken    bool
		wantRequests int
		wantWaits    []time.Duration
		wantErr      []string
		rejectErr    string
	}{
		{
			name:         "throttle then approval completes the login",
			replies:      []exchangeReply{throttled(""), approved()},
			wantToken:    true,
			wantRequests: 2,
			wantWaits:    []time.Duration{2 * time.Second},
		},
		{
			name:         "Retry-After longer than the backoff sets the wait",
			replies:      []exchangeReply{throttled("7"), throttled("7"), approved()},
			wantToken:    true,
			wantRequests: 3,
			wantWaits:    []time.Duration{7 * time.Second, 7 * time.Second},
		},
		{
			name:         "backoff grows when the throttle sends no Retry-After",
			replies:      []exchangeReply{throttled(""), throttled(""), throttled(""), approved()},
			wantToken:    true,
			wantRequests: 4,
			wantWaits:    []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second},
		},
		{
			name:         "Retry-After beyond the code lifetime fails without waiting",
			replies:      []exchangeReply{throttled("900")},
			wantRequests: 1,
			wantErr:      []string{"rate limited", "Wait 15m0s", "run the login command again"},
		},
		{
			name:         "Retry-After waits that add up past the code lifetime stop retrying",
			replies:      []exchangeReply{throttled("240"), throttled("240"), throttled("240"), approved()},
			wantRequests: 3,
			wantWaits:    []time.Duration{240 * time.Second, 240 * time.Second},
			wantErr:      []string{"rate limited", "Wait 4m0s", "run the login command again"},
		},
		{
			name:         "retries run out with the throttle message",
			replies:      alwaysThrottled,
			wantRequests: maxCodeExchangeAttempts,
			wantWaits:    []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second},
			wantErr:      []string{"rate limited", "Wait a few minutes", "run the login command again"},
			rejectErr:    "token exchange failed (HTTP 429)",
		},
		{
			name:         "server fault then approval completes the login",
			replies:      []exchangeReply{{status: http.StatusServiceUnavailable, body: "unavailable"}, approved()},
			wantToken:    true,
			wantRequests: 2,
			wantWaits:    []time.Duration{2 * time.Second},
		},
		{
			name:         "request timeout status then approval completes the login",
			replies:      []exchangeReply{{status: http.StatusRequestTimeout}, approved()},
			wantToken:    true,
			wantRequests: 2,
			wantWaits:    []time.Duration{2 * time.Second},
		},
		{
			name:         "server faults that never clear tell the user to try again",
			replies:      []exchangeReply{{status: http.StatusBadGateway, body: "bad gateway"}},
			wantRequests: maxCodeExchangeAttempts,
			wantWaits:    []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second},
			wantErr:      []string{"token exchange failed (HTTP 502)", "wait a moment, then run the login command again"},
		},
		{
			name:         "connection lost before a response is retried",
			replies:      []exchangeReply{{hangUp: true}, approved()},
			wantToken:    true,
			wantRequests: 2,
			wantWaits:    []time.Duration{2 * time.Second},
		},
		{
			name:         "invalid_grant is not retried",
			replies:      []exchangeReply{{status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`}, approved()},
			wantRequests: 1,
			wantErr:      []string{"token exchange failed: invalid_grant (HTTP 400)"},
		},
		{
			name:         "a throttle status that carries an OAuth error is not retried",
			replies:      []exchangeReply{{status: http.StatusTooManyRequests, body: `{"error":"invalid_grant"}`}, approved()},
			wantRequests: 1,
			wantErr:      []string{"token exchange failed: invalid_grant (HTTP 429)"},
		},
		{
			name:         "a status outside the retry list is not retried",
			replies:      []exchangeReply{{status: http.StatusForbidden, body: "forbidden"}, approved()},
			wantRequests: 1,
			wantErr:      []string{"token exchange failed (HTTP 403)"},
		},
		{
			name:         "an approval with a body we cannot use is not retried",
			replies:      []exchangeReply{{status: http.StatusOK, body: "not json"}, approved()},
			wantRequests: 1,
			wantErr:      []string{"could not parse token response"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests int
			srv := codeExchangeServer(t, tt.replies, &requests)
			defer srv.Close()

			cfg := exchangeTestConfig(srv)
			var clock fakeClock
			clock.configure(&cfg)

			result, err := exchangeCodeResult(context.Background(), cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier")

			if tt.wantToken {
				if err != nil {
					t.Fatalf("exchangeCodeResult: %v", err)
				}
				if result.AccessToken != "exchanged-token" {
					t.Fatalf("access token = %q, want exchanged-token", result.AccessToken)
				}
			} else {
				if err == nil {
					t.Fatal("expected an error")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("error %q does not contain %q", err, want)
					}
				}
				if tt.rejectErr != "" && strings.Contains(err.Error(), tt.rejectErr) {
					t.Fatalf("error %q still reads as the bare %q", err, tt.rejectErr)
				}
			}
			if requests != tt.wantRequests {
				t.Fatalf("got %d token requests, want %d", requests, tt.wantRequests)
			}
			if fmt.Sprint(clock.waits) != fmt.Sprint(tt.wantWaits) {
				t.Fatalf("got waits %v, want %v", clock.waits, tt.wantWaits)
			}
		})
	}
}

func TestExchangeCodeResult_CancelDuringThrottleWaitStops(t *testing.T) {
	var requests int
	srv := codeExchangeServer(t, []exchangeReply{throttled("30"), approved()}, &requests)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cfg := exchangeTestConfig(srv)
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	_, err := exchangeCodeResult(ctx, cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier")
	if err == nil || !strings.Contains(err.Error(), "authorization cancelled") {
		t.Fatalf("got error %v, want authorization cancelled", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want it to wrap context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancellation took %s, want it to interrupt the 30s throttle wait", elapsed)
	}
	if requests != 1 {
		t.Fatalf("got %d token requests, want 1", requests)
	}
}

func TestExchangeCodeResult_DebugLogsAttempts(t *testing.T) {
	var requests int
	srv := codeExchangeServer(t, []exchangeReply{throttled("3"), approved()}, &requests)
	defer srv.Close()

	var debug strings.Builder
	cfg := exchangeTestConfig(srv)
	cfg.DebugWriter = &debug
	var clock fakeClock
	clock.configure(&cfg)

	if _, err := exchangeCodeResult(context.Background(), cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier"); err != nil {
		t.Fatalf("exchangeCodeResult: %v", err)
	}
	for _, want := range []string{
		"token exchange attempt=1 outcome=transient_error retrying wait=3s retry_after=3s",
		"token exchange attempt=2 outcome=approved",
	} {
		if !strings.Contains(debug.String(), want) {
			t.Fatalf("debug output missing %q:\n%s", want, debug.String())
		}
	}
}

func TestBrowserFlow_ThrottledExchangeOutlivesAuthorizationTimeout(t *testing.T) {
	var requests int
	srv := codeExchangeServer(t, []exchangeReply{throttled("1"), approved()}, &requests)
	defer srv.Close()

	cfg := exchangeTestConfig(srv)
	cfg.AuthorizeURL = "http://example.com/oauth/authorize"
	cfg.Timeout = 200 * time.Millisecond
	cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		return sleepContext(ctx, cfg.Timeout+100*time.Millisecond)
	}

	result, err := BrowserFlowResult(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		callbackURL := fmt.Sprintf("%s?code=auth-code&state=%s", u.Query().Get("redirect_uri"), u.Query().Get("state"))
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err != nil {
		t.Fatalf("BrowserFlowResult should finish a throttled exchange after the authorization wait ends, got: %v", err)
	}
	if result.AccessToken != "exchanged-token" {
		t.Fatalf("access token = %q, want exchanged-token", result.AccessToken)
	}
	if requests != 2 {
		t.Fatalf("got %d token requests, want 2 (throttled, approved)", requests)
	}
}

func TestExchangeCodeResult_ReportsRetryWaitsToStatusWriter(t *testing.T) {
	var requests int
	srv := codeExchangeServer(t, []exchangeReply{throttled("240"), {status: http.StatusServiceUnavailable}, approved()}, &requests)
	defer srv.Close()

	var status strings.Builder
	cfg := exchangeTestConfig(srv)
	cfg.StatusWriter = &status
	var clock fakeClock
	clock.configure(&cfg)

	if _, err := exchangeCodeResult(context.Background(), cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier"); err != nil {
		t.Fatalf("exchangeCodeResult: %v", err)
	}
	want := "Gumroad is rate limiting login requests; retrying in 4m0s...\nLogin request failed; retrying in 4s...\n"
	if status.String() != want {
		t.Fatalf("status output = %q, want %q", status.String(), want)
	}
	if strings.Contains(status.String(), "DEBUG") {
		t.Fatalf("status output contains debug lines: %q", status.String())
	}
}

func TestExchangeCodeResult_UnreadableThrottledBodyHonorsRetryAfter(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			mustEncode(t, w, TokenResponse{AccessToken: "exchanged-token", TokenType: "Bearer"})
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = bufrw.WriteString("HTTP/1.1 429 Too Many Requests\r\nRetry-After: 9\r\nContent-Length: 500\r\n\r\nRetry later")
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()

	cfg := exchangeTestConfig(srv)
	var clock fakeClock
	clock.configure(&cfg)

	result, err := exchangeCodeResult(context.Background(), cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier")
	if err != nil {
		t.Fatalf("exchangeCodeResult: %v", err)
	}
	if result.AccessToken != "exchanged-token" {
		t.Fatalf("access token = %q, want exchanged-token", result.AccessToken)
	}
	if fmt.Sprint(clock.waits) != fmt.Sprint([]time.Duration{9 * time.Second}) {
		t.Fatalf("got waits %v, want [9s] from the unreadable throttle's Retry-After", clock.waits)
	}
}

func TestExchangeCodeResult_UnreadableApprovalIsNotRetried(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 500\r\n\r\n{\"access_")
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()

	cfg := exchangeTestConfig(srv)
	var clock fakeClock
	clock.configure(&cfg)

	_, err := exchangeCodeResult(context.Background(), cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier")
	if err == nil || !strings.Contains(err.Error(), "could not read token response") {
		t.Fatalf("got error %v, want could not read token response", err)
	}
	if requests != 1 {
		t.Fatalf("got %d token requests, want 1 (the code may already be spent)", requests)
	}
}

func TestExchangeCodeResult_UnreadableThrottleWithOAuthErrorIsNotRetried(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = bufrw.WriteString("HTTP/1.1 429 Too Many Requests\r\nContent-Type: application/json\r\nRetry-After: 5\r\nContent-Length: 500\r\n\r\n{\"error\":\"invalid_grant\"}")
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()

	cfg := exchangeTestConfig(srv)
	var clock fakeClock
	clock.configure(&cfg)

	_, err := exchangeCodeResult(context.Background(), cfg, callbackPayload{Code: "auth-code"}, "http://127.0.0.1/callback", "verifier")
	if err == nil || !strings.Contains(err.Error(), "token exchange failed: invalid_grant (HTTP 429)") {
		t.Fatalf("got error %v, want the invalid_grant OAuth error", err)
	}
	if requests != 1 || len(clock.waits) != 0 {
		t.Fatalf("got %d requests and waits %v, want 1 request and no wait for an OAuth error", requests, clock.waits)
	}
}
