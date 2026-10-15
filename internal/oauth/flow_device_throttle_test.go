package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func deviceCodeResponse() DeviceCodeResponse {
	return DeviceCodeResponse{
		DeviceCode:      "device-code-123",
		UserCode:        "GRD-ABCD-1234",
		VerificationURI: "https://gumroad.com/oauth/device",
		ExpiresIn:       600,
		Interval:        1,
	}
}

// deviceThrottleServer approves the login after throttledPolls token polls that
// answer the way the app's Rack::Attack throttle does: a plain-text status with
// no OAuth error JSON, optionally carrying Retry-After.
func deviceThrottleServer(t *testing.T, throttledPolls int, retryAfter string, status int, polls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			mustEncode(t, w, deviceCodeResponse())
		case "/oauth/token":
			*polls++
			if *polls <= throttledPolls {
				if retryAfter != "" {
					w.Header().Set("Retry-After", retryAfter)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("Retry later\n"))
				return
			}
			mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestDeviceFlow_RetriesThrottledPollHonoringRetryAfter(t *testing.T) {
	var polls int
	srv := deviceThrottleServer(t, 2, "7", http.StatusTooManyRequests, &polls)
	defer srv.Close()

	var waits []time.Duration
	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}

	result, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{})
	if err != nil {
		t.Fatalf("DeviceFlowResult should survive a throttled poll, got: %v", err)
	}
	if result.AccessToken != "device-access-token" {
		t.Fatalf("got access token %q, want device-access-token", result.AccessToken)
	}
	if polls != 3 {
		t.Fatalf("got %d token polls, want 3 (throttled, throttled, approved)", polls)
	}
	if len(waits) != 3 {
		t.Fatalf("got %d waits, want 3", len(waits))
	}
	if waits[0] != time.Second {
		t.Fatalf("first wait = %s, want the flow interval 1s before any throttle", waits[0])
	}
	for i, wait := range waits[1:] {
		if wait < 7*time.Second {
			t.Fatalf("wait %d = %s, want at least the Retry-After the server sent (7s)", i+1, wait)
		}
	}
}

func TestDeviceFlow_RetriesThrottledPollWithoutRetryAfter(t *testing.T) {
	var polls int
	srv := deviceThrottleServer(t, 1, "", http.StatusTooManyRequests, &polls)
	defer srv.Close()

	var waits []time.Duration
	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}

	if _, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{}); err != nil {
		t.Fatalf("DeviceFlowResult should survive a throttled poll with no Retry-After, got: %v", err)
	}
	if polls != 2 {
		t.Fatalf("got %d token polls, want 2 (throttled, approved)", polls)
	}
	if len(waits) != 2 || waits[1] != time.Second {
		t.Fatalf("got waits %v, want the flow interval 1s on both polls", waits)
	}
}

func TestDeviceFlow_RetriesEveryRetryableStatus(t *testing.T) {
	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var polls int
			srv := deviceThrottleServer(t, 1, "", status, &polls)
			defer srv.Close()

			result, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &strings.Builder{})
			if err != nil {
				t.Fatalf("DeviceFlowResult should survive a %d poll, got: %v", status, err)
			}
			if result.AccessToken != "device-access-token" {
				t.Fatalf("got access token %q, want device-access-token", result.AccessToken)
			}
			if polls != 2 {
				t.Fatalf("got %d token polls, want 2 (%d, approved)", polls, status)
			}
		})
	}
}

func TestDeviceFlow_ThrottleWaitDoesNotPersist(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/device/code":
			mustEncode(t, w, deviceCodeResponse())
		case "/oauth/token":
			polls++
			switch polls {
			case 1:
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("Retry later\n"))
			case 2:
				w.WriteHeader(http.StatusBadRequest)
				mustEncode(t, w, oauthErrorResponse{Error: "authorization_pending"})
			default:
				mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer"})
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var waits []time.Duration
	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}

	if _, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{}); err != nil {
		t.Fatalf("DeviceFlowResult: %v", err)
	}
	want := []time.Duration{time.Second, 7 * time.Second, time.Second}
	if len(waits) != len(want) {
		t.Fatalf("got waits %v, want %v", waits, want)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("got waits %v, want %v", waits, want)
		}
	}
}

func TestDeviceFlow_UnreadableThrottledBodyHonorsRetryAfter(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			w.Header().Set("Content-Type", "application/json")
			mustEncode(t, w, deviceCodeResponse())
		case "/oauth/token":
			polls++
			if polls == 1 {
				// The declared Content-Length is larger than the body, so the
				// read fails after the status and Retry-After already arrived.
				hj, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("response writer does not support hijacking")
				}
				conn, bufrw, err := hj.Hijack()
				if err != nil {
					t.Fatalf("hijack: %v", err)
				}
				_, _ = bufrw.WriteString("HTTP/1.1 429 Too Many Requests\r\nContent-Type: text/plain\r\nRetry-After: 9\r\nContent-Length: 500\r\n\r\nRetry later\n")
				_ = bufrw.Flush()
				_ = conn.Close()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var waits []time.Duration
	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}

	if _, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{}); err != nil {
		t.Fatalf("DeviceFlowResult should survive an unreadable throttled body, got: %v", err)
	}
	if polls != 2 {
		t.Fatalf("got %d token polls, want 2 (unreadable 429, approved)", polls)
	}
	if len(waits) != 2 || waits[1] < 9*time.Second {
		t.Fatalf("got waits %v, want the second wait to honor Retry-After 9s", waits)
	}
}

func TestDeviceFlow_NonRetryableStatusFailsFast(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			mustEncode(t, w, deviceCodeResponse())
		case "/oauth/token":
			polls++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Forbidden\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &strings.Builder{})
	if err == nil {
		t.Fatal("expected error for a non-retryable token endpoint status")
	}
	if !strings.Contains(err.Error(), "token exchange failed (HTTP 403)") {
		t.Fatalf("got error %q, want token exchange failed (HTTP 403)", err)
	}
	if polls != 1 {
		t.Fatalf("got %d token polls, want 1 (a 403 must not retry)", polls)
	}
}

// devicePollScriptServer answers each token poll with the next step of the
// script, repeating the last step once the script runs out.
func devicePollScriptServer(t *testing.T, retryAfter string, script ...string) *httptest.Server {
	t.Helper()
	polls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			mustEncode(t, w, deviceCodeResponse())
		case "/oauth/token":
			step := script[min(polls, len(script)-1)]
			polls++
			switch step {
			case "429":
				if retryAfter != "" {
					w.Header().Set("Retry-After", retryAfter)
				}
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("Retry later\n"))
			case "503":
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("Unavailable\n"))
			case "pending", "expired":
				code := map[string]string{"pending": "authorization_pending", "expired": "expired_token"}[step]
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				mustEncode(t, w, oauthErrorResponse{Error: code})
			case "approved":
				mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer"})
			default:
				t.Errorf("unknown script step %q", step)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func rateLimitLines(status string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if strings.HasPrefix(line, "Gumroad is rate limiting login requests from this network address") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestDeviceFlow_ReportsThrottleOncePerEpisode(t *testing.T) {
	tests := []struct {
		name   string
		script []string
		lines  int
	}{
		{"one run of throttled polls", []string{"503", "429", "429", "429", "503", "approved"}, 1},
		{"two runs split by a pending answer", []string{"429", "429", "pending", "429", "approved"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := devicePollScriptServer(t, "60", tt.script...)
			defer srv.Close()

			var status strings.Builder
			cfg := deviceFlowConfig(srv)
			cfg.StatusWriter = &status

			if _, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{}); err != nil {
				t.Fatalf("DeviceFlowResult: %v", err)
			}
			lines := rateLimitLines(status.String())
			if len(lines) != tt.lines {
				t.Fatalf("got %d rate limit lines, want %d; status output:\n%s", len(lines), tt.lines, status.String())
			}
			if strings.Count(status.String(), "\n") != tt.lines {
				t.Fatalf("got status output other than the rate limit lines (5xx retries must stay quiet):\n%s", status.String())
			}
			for _, line := range lines {
				if !strings.Contains(line, "still waiting for approval and retrying in 1m0s...") {
					t.Fatalf("got line %q, want it to say the login is still waiting and when it retries", line)
				}
			}
		})
	}
}

func TestDeviceFlow_ExpiryNamesRateLimitOnlyAfterThrottledPoll(t *testing.T) {
	tests := []struct {
		name          string
		script        []string
		wantRateLimit bool
	}{
		{"throttled until the code expires", []string{"429"}, true},
		{"pending until the code expires", []string{"pending"}, false},
		{"server expires the code after a throttled poll", []string{"429", "pending", "expired"}, true},
		{"server expires the code without a throttled poll", []string{"pending", "expired"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := devicePollScriptServer(t, "60", tt.script...)
			defer srv.Close()

			cfg := deviceFlowConfig(srv)
			var clock fakeClock
			clock.configure(&cfg)
			advance := cfg.Sleep
			cfg.Sleep = func(ctx context.Context, d time.Duration) error {
				if len(clock.waits) >= 1000 {
					return context.Canceled
				}
				return advance(ctx, d)
			}

			_, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{})
			if err == nil {
				t.Fatal("expected the login to expire")
			}
			if !strings.HasPrefix(err.Error(), "authorization expired") {
				t.Fatalf("got error %q, want it to start with authorization expired", err)
			}
			if got := errors.Is(err, ErrDeviceLoginRateLimited); got != tt.wantRateLimit {
				t.Fatalf("errors.Is(err, ErrDeviceLoginRateLimited) = %v, want %v (error %q)", got, tt.wantRateLimit, err)
			}
			if tt.wantRateLimit && !strings.Contains(err.Error(), "Gumroad rate limited the login polls from this network address") {
				t.Fatalf("got error %q, want it to name the rate limit", err)
			}
			if !tt.wantRateLimit && err.Error() != "authorization expired" {
				t.Fatalf("got error %q, want the bare authorization expired", err)
			}
		})
	}
}
