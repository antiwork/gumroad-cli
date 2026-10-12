package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// exchangeThrottleServer answers the first throttledExchanges token requests
// the way an edge throttle does — a plain-text status with no OAuth error JSON,
// optionally carrying Retry-After — then approves the exchange.
func exchangeThrottleServer(t *testing.T, throttledExchanges int, retryAfter string, status int, exchanges *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*exchanges++
		if *exchanges <= throttledExchanges {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte("Retry later\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		mustEncode(t, w, TokenResponse{AccessToken: "browser-access-token", TokenType: "Bearer"})
	}))
}

// exchangeConfig is testConfig with the token server's URL and a recording
// sleep, so a test can assert the waits a throttle produced without spending
// them.
func exchangeConfig(srv *httptest.Server, waits *[]time.Duration) FlowConfig {
	cfg := testConfig(srv)
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	}
	return cfg
}

// authorizeInBrowser drives the browser flow the way a seller does: it
// completes authorization through the redirect the flow printed.
func authorizeInBrowser(t *testing.T, cfg FlowConfig) (FlowResult, error) {
	t.Helper()
	return BrowserFlowResult(context.Background(), cfg, func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		callbackURL := fmt.Sprintf("%s?code=auth-code&state=%s", u.Query().Get("redirect_uri"), u.Query().Get("state"))
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
}

func TestBrowserFlow_RetriesThrottledExchangeHonoringRetryAfter(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 2, "7", http.StatusTooManyRequests, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	result, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err != nil {
		t.Fatalf("browser login should survive a throttled exchange, got: %v", err)
	}
	if result.AccessToken != "browser-access-token" {
		t.Fatalf("got access token %q, want browser-access-token", result.AccessToken)
	}
	if exchanges != 3 {
		t.Fatalf("got %d token exchanges, want 3 (throttled, throttled, approved)", exchanges)
	}
	if len(waits) != 2 {
		t.Fatalf("got %d waits, want 2", len(waits))
	}
	for i, wait := range waits {
		if wait < 7*time.Second {
			t.Fatalf("wait %d = %s, want at least the Retry-After the server sent (7s)", i+1, wait)
		}
	}
}

func TestBrowserFlow_RetriesEveryRetryableExchangeStatus(t *testing.T) {
	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var exchanges int
			srv := exchangeThrottleServer(t, 1, "", status, &exchanges)
			defer srv.Close()

			var waits []time.Duration
			result, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
			if err != nil {
				t.Fatalf("browser login should survive a %d exchange, got: %v", status, err)
			}
			if result.AccessToken != "browser-access-token" {
				t.Fatalf("got access token %q, want browser-access-token", result.AccessToken)
			}
			if exchanges != 2 {
				t.Fatalf("got %d token exchanges, want 2 (%d, approved)", exchanges, status)
			}
		})
	}
}

func TestBrowserFlow_ExchangeBacksOffWhenThrottleNamesNoWait(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 1, "", http.StatusTooManyRequests, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	if _, err := authorizeInBrowser(t, exchangeConfig(srv, &waits)); err != nil {
		t.Fatalf("browser login should survive a throttle with no Retry-After, got: %v", err)
	}
	if len(waits) != 1 || waits[0] != exchangeRetryBackoff {
		t.Fatalf("got waits %v, want a single %s backoff", waits, exchangeRetryBackoff)
	}
}

func TestBrowserFlow_RetriesTransportFailureOnExchange(t *testing.T) {
	exchanges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		if exchanges == 1 {
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
		w.Header().Set("Content-Type", "application/json")
		mustEncode(t, w, TokenResponse{AccessToken: "browser-access-token", TokenType: "Bearer"})
	}))
	defer srv.Close()

	var waits []time.Duration
	result, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err != nil {
		t.Fatalf("browser login should survive a dropped exchange connection, got: %v", err)
	}
	if result.AccessToken != "browser-access-token" {
		t.Fatalf("got access token %q, want browser-access-token", result.AccessToken)
	}
	if exchanges != 2 {
		t.Fatalf("got %d token exchanges, want 2 (connection dropped, approved)", exchanges)
	}
}

func TestBrowserFlow_ThrottleBeyondBudgetIsReportedNotWaited(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 1, "3600", http.StatusTooManyRequests, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	_, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err == nil {
		t.Fatal("expected the login to report a throttle it cannot wait out")
	}
	if !strings.Contains(err.Error(), "token exchange failed (HTTP 429)") {
		t.Fatalf("got error %q, want it to keep the status", err)
	}
	if !strings.Contains(err.Error(), "rate limiting") {
		t.Fatalf("got error %q, want it to name the throttle", err)
	}
	if exchanges != 1 {
		t.Fatalf("got %d token exchanges, want 1 (an hour-long wait is not taken)", exchanges)
	}
	if len(waits) != 0 {
		t.Fatalf("got waits %v, want none", waits)
	}
}

func TestBrowserFlow_ExchangeGivesUpAfterTheBudget(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 100, "6", http.StatusTooManyRequests, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	_, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err == nil {
		t.Fatal("expected the login to give up once the retry budget is spent")
	}
	if !strings.Contains(err.Error(), "rate limiting") {
		t.Fatalf("got error %q, want it to name the throttle", err)
	}
	waited := time.Duration(0)
	for _, wait := range waits {
		waited += wait
	}
	if waited != exchangeRetryBudget {
		t.Fatalf("got waits %v totalling %s, want exactly the %s budget", waits, waited, exchangeRetryBudget)
	}
	if exchanges != len(waits)+1 {
		t.Fatalf("got %d token exchanges for %d waits, want one more exchange than waits", exchanges, len(waits))
	}
}

func TestBrowserFlow_NonRetryableExchangeStatusFailsFast(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 1, "", http.StatusForbidden, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	_, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err == nil {
		t.Fatal("expected an error for a non-retryable token endpoint status")
	}
	if !strings.Contains(err.Error(), "token exchange failed (HTTP 403)") {
		t.Fatalf("got error %q, want token exchange failed (HTTP 403)", err)
	}
	if exchanges != 1 {
		t.Fatalf("got %d token exchanges, want 1 (a 403 must not retry)", exchanges)
	}
}

func TestBrowserFlow_UnreadableSuccessBodyDoesNotRetry(t *testing.T) {
	exchanges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		// A 200 whose declared Content-Length overshoots the body: the app
		// issued the token and spent the code, so the read failure is final.
		hijackShortBody(t, w, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 500\r\n\r\n{\"access_token\":")
	}))
	defer srv.Close()

	var waits []time.Duration
	_, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err == nil {
		t.Fatal("expected an error for an unreadable token response")
	}
	if !strings.Contains(err.Error(), "could not read token response") {
		t.Fatalf("got error %q, want a read failure", err)
	}
	if exchanges != 1 {
		t.Fatalf("got %d token exchanges, want 1 (the code is already spent)", exchanges)
	}
}

func TestBrowserFlow_UnreadableThrottledBodyHonorsRetryAfter(t *testing.T) {
	exchanges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		if exchanges == 1 {
			// The declared Content-Length overshoots the body, so the read
			// fails after the status and Retry-After already arrived.
			hijackShortBody(t, w, "HTTP/1.1 429 Too Many Requests\r\nContent-Type: text/plain\r\nRetry-After: 9\r\nContent-Length: 500\r\n\r\nRetry later\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		mustEncode(t, w, TokenResponse{AccessToken: "browser-access-token", TokenType: "Bearer"})
	}))
	defer srv.Close()

	var waits []time.Duration
	if _, err := authorizeInBrowser(t, exchangeConfig(srv, &waits)); err != nil {
		t.Fatalf("browser login should survive an unreadable throttled body, got: %v", err)
	}
	if exchanges != 2 {
		t.Fatalf("got %d token exchanges, want 2 (unreadable 429, approved)", exchanges)
	}
	if len(waits) != 1 || waits[0] < 9*time.Second {
		t.Fatalf("got waits %v, want the wait to honor Retry-After 9s", waits)
	}
}

func TestBrowserFlow_ServerFaultGiveUpDoesNotClaimAThrottle(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 100, "60", http.StatusServiceUnavailable, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	_, err := authorizeInBrowser(t, exchangeConfig(srv, &waits))
	if err == nil {
		t.Fatal("expected the login to give up on a server fault it cannot wait out")
	}
	if !strings.Contains(err.Error(), "token exchange failed (HTTP 503)") {
		t.Fatalf("got error %q, want it to keep the status", err)
	}
	if strings.Contains(err.Error(), "rate limiting") {
		t.Fatalf("got error %q, want no throttle claim for a server fault", err)
	}
}

// hijackShortBody answers with a raw response whose declared Content-Length
// overshoots the body it writes, so the client's read fails after the status
// line and headers already arrived.
func hijackShortBody(t *testing.T, w http.ResponseWriter, raw string) {
	t.Helper()
	hj, ok := w.(http.Hijacker)
	if !ok {
		t.Fatal("response writer does not support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		t.Fatalf("hijack: %v", err)
	}
	_, _ = bufrw.WriteString(raw)
	_ = bufrw.Flush()
	_ = conn.Close()
}

func TestHeadlessFlow_RetriesThrottledExchange(t *testing.T) {
	var exchanges int
	srv := exchangeThrottleServer(t, 1, "", http.StatusTooManyRequests, &exchanges)
	defer srv.Close()

	var waits []time.Duration
	cfg := exchangeConfig(srv, &waits)
	var output strings.Builder

	token, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		state := printedState(t, output.String())
		return fmt.Sprintf("http://127.0.0.1/callback?code=headless-code&state=%s", state), nil
	})
	if err != nil {
		t.Fatalf("headless login should survive a throttled exchange, got: %v", err)
	}
	if token != "browser-access-token" {
		t.Fatalf("token = %q, want browser-access-token", token)
	}
	if exchanges != 2 {
		t.Fatalf("got %d token exchanges, want 2 (throttled, approved)", exchanges)
	}
}

// printedState reads the state the headless flow printed alongside the
// authorize URL, so the pasted redirect is accepted.
func printedState(t *testing.T, printed string) string {
	t.Helper()
	for _, line := range strings.Split(printed, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "http") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil {
			t.Fatalf("parse printed URL: %v", err)
		}
		return u.Query().Get("state")
	}
	t.Fatal("could not find the authorize URL in the flow output")
	return ""
}
