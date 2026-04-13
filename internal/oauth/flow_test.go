package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustEncode(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // G107: test-only, URL is constructed from test server
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func testConfig(tokenServer *httptest.Server) FlowConfig {
	return FlowConfig{
		ClientID:     "test-client-id",
		AuthorizeURL: "http://unused/oauth/authorize",
		TokenURL:     tokenServer.URL + "/oauth/token",
		Scopes:       "edit_products view_sales",
		Timeout:      5 * time.Second,
		HTTPClient:   tokenServer.Client(),
	}
}

func tokenHandler(t *testing.T, wantVerifier bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		if r.Method != "POST" {
			t.Errorf("token request method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %s, want application/x-www-form-urlencoded", ct)
		}

		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))

		if vals.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", vals.Get("grant_type"))
		}
		if vals.Get("client_id") != "test-client-id" {
			t.Errorf("client_id = %q, want test-client-id", vals.Get("client_id"))
		}
		if wantVerifier && vals.Get("code_verifier") == "" {
			t.Error("code_verifier is missing from token request")
		}

		w.Header().Set("Content-Type", "application/json")
		mustEncode(t, w, TokenResponse{
			AccessToken: "test-access-token",
			TokenType:   "bearer",
			Scope:       "edit_products view_sales",
		})
	}
}

func TestBrowserFlow_HappyPath(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, true))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	token, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		// Simulate browser: parse the authorize URL, extract state, hit the callback.
		u, _ := url.Parse(authURL)
		state := u.Query().Get("state")
		redirectURI := u.Query().Get("redirect_uri")
		if state == "" {
			t.Fatal("state missing from authorize URL")
		}

		// Hit the callback endpoint.
		callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err != nil {
		t.Fatalf("BrowserFlow: %v", err)
	}
	if token != "test-access-token" {
		t.Fatalf("token = %q, want test-access-token", token)
	}
}

func TestBrowserFlow_StateMismatch(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)
	cfg.Timeout = 200 * time.Millisecond

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")

		// Send a callback with wrong state — should be silently ignored.
		callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=wrong-state", redirectURI)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for wrong state, got %d", resp.StatusCode)
		}
		return nil
	})
	// Flow should time out since the wrong-state callback is ignored.
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error (invalid callback ignored), got: %v", err)
	}
}

func TestBrowserFlow_UserDenied(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")

		callbackURL := fmt.Sprintf("%s?error=access_denied&error_description=User+denied&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("expected authorization denied error, got: %v", err)
	}
}

func TestBrowserFlow_Timeout(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)
	cfg.Timeout = 100 * time.Millisecond

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		// Don't hit the callback — let it time out.
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestBrowserFlow_BrowserOpenFails(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		return fmt.Errorf("no display available")
	})
	if err == nil || !strings.Contains(err.Error(), "could not open browser") {
		t.Fatalf("expected browser open error, got: %v", err)
	}
}

func TestBrowserFlow_TokenExchangeFailure(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		callbackURL := fmt.Sprintf("%s?code=bad-code&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "token exchange failed (HTTP 400)") {
		t.Fatalf("expected token exchange error, got: %v", err)
	}
}

func TestBrowserFlow_PKCEParamsInTokenExchange(t *testing.T) {
	var capturedVerifier string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		capturedVerifier = vals.Get("code_verifier")

		w.Header().Set("Content-Type", "application/json")
		mustEncode(t, w, TokenResponse{AccessToken: "tok", TokenType: "bearer"})
	}))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")

		// Verify PKCE params in authorize URL.
		if u.Query().Get("code_challenge") == "" {
			t.Error("code_challenge missing from authorize URL")
		}
		if u.Query().Get("code_challenge_method") != "S256" {
			t.Errorf("code_challenge_method = %q, want S256", u.Query().Get("code_challenge_method"))
		}

		callbackURL := fmt.Sprintf("%s?code=auth-code&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err != nil {
		t.Fatalf("BrowserFlow: %v", err)
	}
	if capturedVerifier == "" {
		t.Error("code_verifier was not sent in token exchange")
	}
	if len(capturedVerifier) != 43 {
		t.Errorf("code_verifier length = %d, want 43", len(capturedVerifier))
	}
}

// --- Headless flow tests ---

func TestHeadlessFlow_HappyPath(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, true))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	var output strings.Builder
	var capturedState string

	token, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		// Parse the printed URL to get the state.
		lines := strings.Split(output.String(), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "http") {
				u, _ := url.Parse(line)
				capturedState = u.Query().Get("state")
				break
			}
		}
		if capturedState == "" {
			t.Fatal("could not find state in output")
		}
		return fmt.Sprintf("http://127.0.0.1/callback?code=headless-code&state=%s", capturedState), nil
	})
	if err != nil {
		t.Fatalf("HeadlessFlow: %v", err)
	}
	if token != "test-access-token" {
		t.Fatalf("token = %q, want test-access-token", token)
	}
}

func TestHeadlessFlow_StateMismatch(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	var output strings.Builder
	_, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		return "http://127.0.0.1/callback?code=c&state=wrong", nil
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch error, got: %v", err)
	}
}

func TestHeadlessFlow_UserDenied(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	var output strings.Builder
	var capturedState string

	_, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		lines := strings.Split(output.String(), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "http") {
				u, _ := url.Parse(line)
				capturedState = u.Query().Get("state")
				break
			}
		}
		return fmt.Sprintf("http://127.0.0.1/callback?error=access_denied&state=%s", capturedState), nil
	})
	if err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("expected denied error, got: %v", err)
	}
}

func TestHeadlessFlow_ReadError(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	var output strings.Builder
	_, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		return "", fmt.Errorf("connection closed")
	})
	if err == nil || !strings.Contains(err.Error(), "could not read URL") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestHeadlessFlow_InvalidURL(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	var output strings.Builder
	_, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		return "://bad-url", nil
	})
	if err == nil || !strings.Contains(err.Error(), "invalid URL") {
		t.Fatalf("expected invalid URL error, got: %v", err)
	}
}

func TestHeadlessFlow_NoCode(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	var output strings.Builder
	_, err := HeadlessFlow(context.Background(), cfg, &output, func() (string, error) {
		lines := strings.Split(output.String(), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "http") {
				u, _ := url.Parse(line)
				state := u.Query().Get("state")
				return fmt.Sprintf("http://127.0.0.1/callback?state=%s", state), nil
			}
		}
		return "http://127.0.0.1/callback", nil
	})
	if err == nil || !strings.Contains(err.Error(), "no authorization code") {
		t.Fatalf("expected no code error, got: %v", err)
	}
}

func TestBrowserFlow_NoCode(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")

		callbackURL := fmt.Sprintf("%s?state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "no authorization code") {
		t.Fatalf("expected no code error, got: %v", err)
	}
}

func TestBrowserFlow_TokenResponseEmpty(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"","token_type":"bearer"}`)
	}))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		callbackURL := fmt.Sprintf("%s?code=c&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "did not contain an access token") {
		t.Fatalf("expected empty token error, got: %v", err)
	}
}

func TestBrowserFlow_TokenResponseInvalidJSON(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		callbackURL := fmt.Sprintf("%s?code=c&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "could not parse token response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestDefaultFlowConfig(t *testing.T) {
	cfg := DefaultFlowConfig()
	if cfg.ClientID != ClientID {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, ClientID)
	}
	if cfg.AuthorizeURL != AuthorizeURL {
		t.Errorf("AuthorizeURL = %q, want %q", cfg.AuthorizeURL, AuthorizeURL)
	}
	if cfg.TokenURL != TokenURL {
		t.Errorf("TokenURL = %q, want %q", cfg.TokenURL, TokenURL)
	}
	if cfg.Scopes != Scopes {
		t.Errorf("Scopes = %q, want %q", cfg.Scopes, Scopes)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
}

func TestBrowserFlow_ErrorParamWithoutDescription(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, false))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	_, err := BrowserFlow(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		callbackURL := fmt.Sprintf("%s?error=server_error&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "server_error") {
		t.Fatalf("expected server_error in message, got: %v", err)
	}
}
