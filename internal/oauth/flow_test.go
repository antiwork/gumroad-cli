package oauth

import (
	"context"
	"encoding/json"
	"errors"
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

func TestBrowserFlowResult_ReturnsAdminCodeFromUnifiedCallback(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenHandler(t, true))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)
	cfg.OptionalAdmin = true

	result, err := BrowserFlowResult(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		if u.Query().Get("admin_scope") != "optional" {
			t.Fatalf("admin_scope = %q, want optional", u.Query().Get("admin_scope"))
		}
		state := u.Query().Get("state")
		redirectURI := u.Query().Get("redirect_uri")
		callbackURL := fmt.Sprintf("%s?code=test-auth-code&admin_code=admin-auth-code&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err != nil {
		t.Fatalf("BrowserFlowResult: %v", err)
	}
	if result.AccessToken != "test-access-token" {
		t.Fatalf("token = %q, want test-access-token", result.AccessToken)
	}
	if result.AdminAuthorizationCode != "admin-auth-code" {
		t.Fatalf("admin code = %q, want admin-auth-code", result.AdminAuthorizationCode)
	}
	if result.CodeVerifier == "" {
		t.Fatal("expected code verifier for admin exchange")
	}
}

func TestBrowserFlowResult_ReturnsAdminTokenFromTokenResponse(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustEncode(t, w, TokenResponse{
			AccessToken: "test-access-token",
			TokenType:   "bearer",
			AdminToken: &AdminTokenResponse{
				Token:           "admin-token",
				TokenExternalID: "adm_123",
				Actor:           AdminActor{Name: "Admin User", Email: "admin@example.com"},
				ExpiresAt:       "2026-06-01T00:00:00Z",
			},
		})
	}))
	defer tokenSrv.Close()
	cfg := testConfig(tokenSrv)

	result, err := BrowserFlowResult(context.Background(), cfg, func(authURL string) error {
		u, _ := url.Parse(authURL)
		state := u.Query().Get("state")
		redirectURI := u.Query().Get("redirect_uri")
		callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", redirectURI, state)
		resp := mustGet(t, callbackURL)
		resp.Body.Close()
		return nil
	})
	if err != nil {
		t.Fatalf("BrowserFlowResult: %v", err)
	}
	if result.AdminToken == nil || result.AdminToken.Token != "admin-token" || result.AdminToken.Actor.Email != "admin@example.com" {
		t.Fatalf("unexpected admin token result: %+v", result.AdminToken)
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
	if err == nil || !strings.Contains(err.Error(), "token exchange failed: invalid_grant (HTTP 400)") {
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

func deviceFlowConfig(srv *httptest.Server) FlowConfig {
	return FlowConfig{
		ClientID:      "test-client-id",
		DeviceCodeURL: srv.URL + "/oauth/device/code",
		TokenURL:      srv.URL + "/oauth/token",
		Scopes:        "view_profile edit_products",
		OptionalAdmin: true,
		HTTPClient:    srv.Client(),
		Sleep: func(context.Context, time.Duration) error {
			return nil
		},
	}
}

func TestDeviceFlow_PollsUntilApproved(t *testing.T) {
	var deviceRequest url.Values
	var tokenRequests []url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/oauth/device/code":
			deviceRequest = r.PostForm
			mustEncode(t, w, DeviceCodeResponse{
				DeviceCode:              "device-code-123",
				UserCode:                "GRD-ABCD-1234",
				VerificationURI:         "https://gumroad.com/oauth/device",
				VerificationURIComplete: "https://gumroad.com/oauth/device?user_code=GRD-ABCD-1234",
				ExpiresIn:               600,
				Interval:                1,
			})
		case "/oauth/token":
			tokenRequests = append(tokenRequests, r.PostForm)
			if len(tokenRequests) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				mustEncode(t, w, oauthErrorResponse{Error: "authorization_pending", ErrorDescription: "Authorization is pending"})
				return
			}
			mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer", Scope: "view_profile"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var output strings.Builder
	result, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), &output)
	if err != nil {
		t.Fatalf("DeviceFlowResult: %v", err)
	}
	if result.AccessToken != "device-access-token" {
		t.Fatalf("got access token %q, want device-access-token", result.AccessToken)
	}
	if deviceRequest.Get("client_id") != "test-client-id" ||
		deviceRequest.Get("scope") != "view_profile edit_products" ||
		deviceRequest.Get("admin_scope") != "optional" {
		t.Fatalf("unexpected device request: %v", deviceRequest)
	}
	if len(tokenRequests) != 2 {
		t.Fatalf("got %d token requests, want 2", len(tokenRequests))
	}
	if tokenRequests[0].Get("grant_type") != DeviceGrantType || tokenRequests[0].Get("device_code") != "device-code-123" {
		t.Fatalf("unexpected token request: %v", tokenRequests[0])
	}
	for _, want := range []string{
		"Open this URL to authorize Gumroad CLI:",
		"https://gumroad.com/oauth/device?user_code=GRD-ABCD-1234",
		"Code: GRD-ABCD-1234",
		"Waiting for approval...",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("device flow output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "Paste the full URL") {
		t.Fatalf("device flow should not ask for redirect URL paste:\n%s", output.String())
	}
}

func TestDeviceFlow_SlowDownUsesServerInterval(t *testing.T) {
	var sleeps []time.Duration

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if len(sleeps) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				mustEncode(t, w, oauthErrorResponse{Error: "slow_down", ErrorDescription: "Polling too quickly", Interval: 7})
				return
			}
			mustEncode(t, w, TokenResponse{AccessToken: "device-access-token", TokenType: "Bearer"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	_, err := DeviceFlowResult(context.Background(), cfg, io.Discard)
	if err != nil {
		t.Fatalf("DeviceFlowResult: %v", err)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 7*time.Second {
		t.Fatalf("got sleeps %v, want [1s 7s]", sleeps)
	}
}

func TestDeviceFlow_SleepErrorPreservesContextCause(t *testing.T) {
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
			t.Fatal("token endpoint should not be reached when sleep is cancelled")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := deviceFlowConfig(srv)
	cfg.Sleep = func(context.Context, time.Duration) error {
		return context.Canceled
	}

	_, err := DeviceFlowResult(context.Background(), cfg, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "authorization cancelled") {
		t.Fatalf("expected authorization cancelled error, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled in error chain, got: %v", err)
	}
}

func TestDeviceFlow_AccessDenied(t *testing.T) {
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
			w.WriteHeader(http.StatusBadRequest)
			mustEncode(t, w, oauthErrorResponse{Error: "access_denied", ErrorDescription: "Access was denied"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("expected authorization denied, got: %v", err)
	}
}

func TestDeviceFlow_DeviceCodeRequestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		mustEncode(t, w, oauthErrorResponse{Error: "unauthorized_client", ErrorDescription: "Client is not allowed to use device authorization"})
	}))
	defer srv.Close()

	_, err := DeviceFlowResult(context.Background(), deviceFlowConfig(srv), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unauthorized_client") {
		t.Fatalf("expected unauthorized_client error, got: %v", err)
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
	if cfg.DeviceCodeURL != DeviceCodeURL {
		t.Errorf("DeviceCodeURL = %q, want %q", cfg.DeviceCodeURL, DeviceCodeURL)
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
