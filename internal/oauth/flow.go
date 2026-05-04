package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenResponse is the JSON body returned by the OAuth token endpoint.
type TokenResponse struct {
	AccessToken string              `json:"access_token"`
	TokenType   string              `json:"token_type"`
	Scope       string              `json:"scope"`
	AdminToken  *AdminTokenResponse `json:"admin_token,omitempty"`
	Admin       *AdminTokenResponse `json:"admin,omitempty"`
}

type AdminTokenResponse struct {
	Token           string     `json:"token"`
	TokenExternalID string     `json:"token_external_id"`
	Actor           AdminActor `json:"actor"`
	ExpiresAt       string     `json:"expires_at"`
}

type AdminActor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type FlowResult struct {
	AccessToken            string
	AdminToken             *AdminTokenResponse
	AdminAuthorizationCode string
	CodeVerifier           string
}

// FlowConfig holds the parameters for an OAuth authorization code flow.
type FlowConfig struct {
	ClientID      string
	AuthorizeURL  string
	TokenURL      string
	Scopes        string
	OptionalAdmin bool
	Timeout       time.Duration
	HTTPClient    *http.Client // optional; defaults to http.DefaultClient
}

// DefaultFlowConfigFunc returns a FlowConfig using the built-in constants.
// Replaceable in tests.
var DefaultFlowConfigFunc = defaultFlowConfig

// DefaultFlowConfig returns a FlowConfig using the built-in constants.
func DefaultFlowConfig() FlowConfig {
	return DefaultFlowConfigFunc()
}

func defaultFlowConfig() FlowConfig {
	return FlowConfig{
		ClientID:      ClientID,
		AuthorizeURL:  AuthorizeURL,
		TokenURL:      TokenURL,
		Scopes:        Scopes,
		OptionalAdmin: true,
		Timeout:       DefaultTimeout,
	}
}

type callbackPayload struct {
	Code      string
	AdminCode string
}

// callbackResult carries the authorization callback payload or error from the callback handler.
type callbackResult struct {
	Payload callbackPayload
	Err     error
}

// BrowserFlow runs the full OAuth authorization code flow with PKCE.
// It binds a local listener, opens the authorize URL via openBrowser,
// waits for the callback, and exchanges the code for an access token.
func BrowserFlow(ctx context.Context, cfg FlowConfig, openBrowser func(string) error) (string, error) {
	result, err := BrowserFlowResult(ctx, cfg, openBrowser)
	if err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

// BrowserFlowResult runs BrowserFlow and returns optional admin credential
// material emitted by the unified Gumroad authorization page.
func BrowserFlowResult(ctx context.Context, cfg FlowConfig, openBrowser func(string) error) (FlowResult, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Bind ephemeral port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return FlowResult{}, fmt.Errorf("could not bind local listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Generate PKCE verifier + challenge.
	verifier, err := GenerateVerifier()
	if err != nil {
		_ = listener.Close()
		return FlowResult{}, fmt.Errorf("could not generate PKCE verifier: %w", err)
	}
	challenge := ChallengeFromVerifier(verifier)

	// Generate state for CSRF protection.
	state, err := generateState()
	if err != nil {
		_ = listener.Close()
		return FlowResult{}, fmt.Errorf("could not generate state: %w", err)
	}

	// Build authorize URL.
	authURL := buildAuthorizeURL(cfg, redirectURI, challenge, state)

	// Start callback server.
	resultCh := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callbackHandler(state, resultCh))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			resultCh <- callbackResult{Err: fmt.Errorf("callback server error: %w", err)}
		}
	}()

	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
		<-serverDone
	}()

	// Open browser.
	if err := openBrowser(authURL); err != nil {
		return FlowResult{}, fmt.Errorf("could not open browser: %w", err)
	}

	// Wait for callback or timeout.
	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			return FlowResult{}, fmt.Errorf("authorization cancelled")
		}
		// Drain resultCh in case the callback arrived at the exact deadline.
		select {
		case result = <-resultCh:
			if result.Err != nil {
				return FlowResult{}, result.Err
			}
			// Original ctx expired; give the token exchange its own deadline.
			xctx, xcancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer xcancel()
			return exchangeCodeResult(xctx, cfg, result.Payload, redirectURI, verifier)
		default:
			return FlowResult{}, fmt.Errorf("authorization timed out after %s", cfg.Timeout)
		}
	}
	if result.Err != nil {
		return FlowResult{}, result.Err
	}

	// Exchange code for token.
	return exchangeCodeResult(ctx, cfg, result.Payload, redirectURI, verifier)
}

// HeadlessFlow runs the OAuth flow without a browser: prints the authorize URL
// and prompts the user to paste the redirect URL.
func HeadlessFlow(ctx context.Context, cfg FlowConfig, out io.Writer, readLine func() (string, error)) (string, error) {
	result, err := HeadlessFlowResult(ctx, cfg, out, readLine)
	if err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

// HeadlessFlowResult is HeadlessFlow with optional admin credential metadata.
func HeadlessFlowResult(ctx context.Context, cfg FlowConfig, out io.Writer, readLine func() (string, error)) (FlowResult, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	verifier, err := GenerateVerifier()
	if err != nil {
		return FlowResult{}, fmt.Errorf("could not generate PKCE verifier: %w", err)
	}
	challenge := ChallengeFromVerifier(verifier)

	state, err := generateState()
	if err != nil {
		return FlowResult{}, fmt.Errorf("could not generate state: %w", err)
	}

	// Use a placeholder redirect URI — user will paste the URL from their browser.
	redirectURI := "http://127.0.0.1/callback"
	authURL := buildAuthorizeURL(cfg, redirectURI, challenge, state)

	fmt.Fprintf(out, "Open this URL in your browser:\n  %s\n\n", authURL)
	fmt.Fprintf(out, "After authorizing, your browser will redirect to a localhost URL\n")
	fmt.Fprintf(out, "(it may show an error page — that's expected).\n\n")
	fmt.Fprintf(out, "Paste the full URL from your browser's address bar: ")

	type lineResult struct {
		line string
		err  error
	}
	lineCh := make(chan lineResult, 1)
	go func() {
		l, e := readLine()
		lineCh <- lineResult{l, e}
	}()

	var line string
	select {
	case res := <-lineCh:
		if res.err != nil {
			return FlowResult{}, fmt.Errorf("could not read URL: %w", res.err)
		}
		line = res.line
	case <-ctx.Done():
		return FlowResult{}, fmt.Errorf("authorization timed out after %s", cfg.Timeout)
	}

	payload, err := parseCallbackPayload(strings.TrimSpace(line), state)
	if err != nil {
		return FlowResult{}, err
	}

	return exchangeCodeResult(ctx, cfg, payload, redirectURI, verifier)
}

func generateState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildAuthorizeURL(cfg FlowConfig, redirectURI, challenge, state string) string {
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {cfg.Scopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	if cfg.OptionalAdmin {
		v.Set("admin_scope", "optional")
	}
	return cfg.AuthorizeURL + "?" + v.Encode()
}

func callbackHandler(expectedState string, resultCh chan<- callbackResult) http.HandlerFunc {
	var once sync.Once
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		q := r.URL.Query()

		// Validate state first to reject forged requests from local attackers.
		// Requests with wrong/missing state are silently ignored so the flow
		// keeps waiting for the real callback.
		if q.Get("state") != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, htmlPage("Something went wrong", "Close this tab and try again.", true))
			return
		}

		// State matches — this is the real callback. Extract the result once.
		payload, err := extractCallbackPayload(q, expectedState)
		once.Do(func() {
			if err != nil {
				resultCh <- callbackResult{Err: err}
			} else {
				resultCh <- callbackResult{Payload: payload}
			}
		})

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, htmlPage("Authorization denied", "Close this tab and try again.", true))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, htmlPage("Authorization complete", "You can return to your terminal and close this tab.", false))
	}
}

func parseCallbackPayload(rawURL, expectedState string) (callbackPayload, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return callbackPayload{}, fmt.Errorf("invalid URL: %w", err)
	}
	return extractCallbackPayload(u.Query(), expectedState)
}

func extractCallbackPayload(q url.Values, expectedState string) (callbackPayload, error) {
	if q.Get("state") != expectedState {
		return callbackPayload{}, fmt.Errorf("state mismatch: possible CSRF attack")
	}

	if errParam := q.Get("error"); errParam != "" {
		desc := q.Get("error_description")
		if desc == "" {
			desc = errParam
		}
		return callbackPayload{}, fmt.Errorf("authorization denied: %s", desc)
	}

	code := q.Get("code")
	if code == "" {
		return callbackPayload{}, fmt.Errorf("no authorization code received")
	}
	adminCode := q.Get("admin_code")
	if adminCode == "" {
		adminCode = q.Get("admin_authorization_code")
	}
	return callbackPayload{Code: code, AdminCode: adminCode}, nil
}

func exchangeCodeResult(ctx context.Context, cfg FlowConfig, payload callbackPayload, redirectURI, verifier string) (FlowResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {payload.Code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return FlowResult{}, fmt.Errorf("could not build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return FlowResult{}, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return FlowResult{}, fmt.Errorf("could not read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return FlowResult{}, fmt.Errorf("token exchange failed (HTTP %d)", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return FlowResult{}, fmt.Errorf("could not parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return FlowResult{}, fmt.Errorf("token response did not contain an access token")
	}

	adminToken := tokenResp.AdminToken
	if adminToken == nil {
		adminToken = tokenResp.Admin
	}
	return FlowResult{
		AccessToken:            tokenResp.AccessToken,
		AdminToken:             adminToken,
		AdminAuthorizationCode: payload.AdminCode,
		CodeVerifier:           verifier,
	}, nil
}

func htmlPage(title, message string, isError bool) string {
	t := html.EscapeString(title)
	m := html.EscapeString(message)
	icon := `<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>`
	iconBg := "#ff90e8"
	if isError {
		icon = `<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>`
		iconBg = "#dc341e"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Gumroad CLI</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;padding:1rem;background:#f4f4f0;color:#000;line-height:1.5}
.card{text-align:center;padding:2rem;background:#fff;border:1px solid #000;border-radius:0.25rem;max-width:24rem;width:100%%}
.icon{display:inline-flex;align-items:center;justify-content:center;width:2.5rem;height:2.5rem;margin-bottom:1rem;background:%s;color:#000;border:1px solid #000;border-radius:999px}
h1{font-size:1.25rem;font-weight:700;margin-bottom:0.25rem}
p{color:#666}
</style>
</head><body><main class="card"><div class="icon">%s</div><h1>%s</h1><p>%s</p></main></body></html>`,
		iconBg, icon, t, m)
}
