package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// deviceRetryServer simulates the field failure from a TLS-intercepting
// middlebox: the first poll returns authorization_pending, the second poll's
// connection is reset mid-flight, and the third poll succeeds (the user
// approved in the browser while the CLI was polling).
func deviceRetryServer(t *testing.T, tokenPolls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")

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
			*tokenPolls++
			switch *tokenPolls {
			case 1:
				w.WriteHeader(http.StatusBadRequest)
				mustEncode(t, w, oauthErrorResponse{Error: "authorization_pending", ErrorDescription: "Authorization is pending"})
			case 2:
				hj, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("response writer does not support hijacking")
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					t.Fatalf("hijack: %v", err)
				}
				_ = conn.Close()
			default:
				mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer"})
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestDeviceFlow_RetriesTransientPollErrors(t *testing.T) {
	var tokenPolls int
	srv := deviceRetryServer(t, &tokenPolls)
	defer srv.Close()

	result, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &strings.Builder{})
	if err != nil {
		t.Fatalf("DeviceFlowResult should survive a transient poll error, got: %v", err)
	}
	if result.AccessToken != "device-access-token" {
		t.Fatalf("got access token %q, want device-access-token", result.AccessToken)
	}
	if tokenPolls != 3 {
		t.Fatalf("got %d token polls, want 3 (pending, transient failure, success)", tokenPolls)
	}
}

func TestDeviceFlow_TransientRetryStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	polls := 0
	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(context.Context, time.Duration) error {
		polls++
		if polls > 2 {
			cancel()
		}
		return nil
	}

	_, err := DeviceFlowResult(ctx, cfg, &strings.Builder{})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	if !strings.Contains(err.Error(), "authorization cancelled") {
		t.Fatalf("got error %q, want authorization cancelled", err)
	}
}

func TestDeviceFlow_PermanentResponseErrorFailsFast(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
			// A 200 with a body missing the access token is a broken
			// server contract, not a transient failure.
			mustEncode(t, w, TokenResponse{TokenType: "Bearer"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &strings.Builder{})
	if err == nil {
		t.Fatal("expected error for token response without access token")
	}
	if !strings.Contains(err.Error(), "did not contain an access token") {
		t.Fatalf("got error %q, want missing access token error", err)
	}
	if polls != 1 {
		t.Fatalf("got %d polls, want 1 (permanent response errors must not retry)", polls)
	}
}

func TestDeviceFlow_ApprovedResponseReadErrorFailsFast(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
			// Promise a 200 body, then cut the connection before delivering
			// it. The server may already have consumed the device code, so
			// the flow must fail with guidance instead of retrying the poll.
			w.Header().Set("Content-Length", "512")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"access_`)); err != nil {
				t.Fatalf("write partial body: %v", err)
			}
			w.(http.Flusher).Flush()
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &strings.Builder{})
	if err == nil {
		t.Fatal("expected error when the approved token response cannot be read")
	}
	if !strings.Contains(err.Error(), "run the login command again") {
		t.Fatalf("got error %q, want guidance to run the login command again", err)
	}
	if polls != 1 {
		t.Fatalf("got %d polls, want 1 (read errors on an approved response must not retry)", polls)
	}
}

func TestDeviceFlow_DebugLogsPollAttempts(t *testing.T) {
	var tokenPolls int
	srv := deviceRetryServer(t, &tokenPolls)
	defer srv.Close()

	var debug strings.Builder
	cfg := deviceFlowConfig(srv)
	cfg.DebugWriter = &debug

	if _, err := DeviceFlowResult(context.Background(), cfg, &strings.Builder{}); err != nil {
		t.Fatalf("DeviceFlowResult: %v", err)
	}

	for _, want := range []string{
		"device code request",
		"attempt=1 outcome=authorization_pending",
		"attempt=2 outcome=transient_error retrying",
		"attempt=3 outcome=approved",
	} {
		if !strings.Contains(debug.String(), want) {
			t.Fatalf("debug output missing %q:\n%s", want, debug.String())
		}
	}
}

func TestDeviceFlow_NoDebugWriterProducesNoDebugOutput(t *testing.T) {
	var tokenPolls int
	srv := deviceRetryServer(t, &tokenPolls)
	defer srv.Close()

	var out strings.Builder
	if _, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &out); err != nil {
		t.Fatalf("DeviceFlowResult: %v", err)
	}
	if strings.Contains(out.String(), "DEBUG") {
		t.Fatalf("user-facing output should not contain debug lines:\n%s", out.String())
	}
}
