package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// deviceThrottleServer approves the login after throttledPolls token polls that
// answer the way the app's Rack::Attack throttle does: a plain-text 429 with no
// OAuth error JSON, optionally carrying Retry-After.
func deviceThrottleServer(t *testing.T, throttledPolls int, retryAfter string, polls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			mustEncode(t, w, DeviceCodeResponse{
				DeviceCode:      "device-code-123",
				UserCode:        "GRD-ABCD-1234",
				VerificationURI: "https://gumroad.com/oauth/device",
				ExpiresIn:       600,
				Interval:        1,
			})
		case "/oauth/token":
			*polls++
			if *polls <= throttledPolls {
				if retryAfter != "" {
					w.Header().Set("Retry-After", retryAfter)
				}
				w.WriteHeader(http.StatusTooManyRequests)
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
	srv := deviceThrottleServer(t, 2, "7", &polls)
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
	srv := deviceThrottleServer(t, 1, "", &polls)
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

func TestDeviceFlow_NonRetryableStatusFailsFast(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			mustEncode(t, w, DeviceCodeResponse{
				DeviceCode:      "device-code-123",
				UserCode:        "GRD-ABCD-1234",
				VerificationURI: "https://gumroad.com/oauth/device",
				ExpiresIn:       600,
				Interval:        1,
			})
		case "/oauth/token":
			polls++
			// A 403 with no OAuth error body is a refusal the server does
			// not intend to clear, so it must not be retried.
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
