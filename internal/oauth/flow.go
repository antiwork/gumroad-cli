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
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// FlowConfig holds the parameters for an OAuth authorization code flow.
type FlowConfig struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	Scopes       string
	Timeout      time.Duration
	HTTPClient   *http.Client // optional; defaults to http.DefaultClient
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
		ClientID:     ClientID,
		AuthorizeURL: AuthorizeURL,
		TokenURL:     TokenURL,
		Scopes:       Scopes,
		Timeout:      DefaultTimeout,
	}
}

// callbackResult carries the authorization code or error from the callback handler.
type callbackResult struct {
	Code string
	Err  error
}

// BrowserFlow runs the full OAuth authorization code flow with PKCE.
// It binds a local listener, opens the authorize URL via openBrowser,
// waits for the callback, and exchanges the code for an access token.
func BrowserFlow(ctx context.Context, cfg FlowConfig, openBrowser func(string) error) (string, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Bind ephemeral port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("could not bind local listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Generate PKCE verifier + challenge.
	verifier, err := GenerateVerifier()
	if err != nil {
		_ = listener.Close()
		return "", fmt.Errorf("could not generate PKCE verifier: %w", err)
	}
	challenge := ChallengeFromVerifier(verifier)

	// Generate state for CSRF protection.
	state, err := generateState()
	if err != nil {
		_ = listener.Close()
		return "", fmt.Errorf("could not generate state: %w", err)
	}

	// Build authorize URL.
	authURL := buildAuthorizeURL(cfg, redirectURI, challenge, state)

	// Start callback server.
	resultCh := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callbackHandler(state, resultCh))
	server := &http.Server{Handler: mux}

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
		return "", fmt.Errorf("could not open browser: %w", err)
	}

	// Wait for callback or timeout.
	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		return "", fmt.Errorf("authorization timed out after %s", cfg.Timeout)
	}
	if result.Err != nil {
		return "", result.Err
	}

	// Exchange code for token.
	return exchangeCode(ctx, cfg, result.Code, redirectURI, verifier)
}

// HeadlessFlow runs the OAuth flow without a browser: prints the authorize URL
// and prompts the user to paste the redirect URL.
func HeadlessFlow(ctx context.Context, cfg FlowConfig, out io.Writer, readLine func() (string, error)) (string, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	verifier, err := GenerateVerifier()
	if err != nil {
		return "", fmt.Errorf("could not generate PKCE verifier: %w", err)
	}
	challenge := ChallengeFromVerifier(verifier)

	state, err := generateState()
	if err != nil {
		return "", fmt.Errorf("could not generate state: %w", err)
	}

	// Use a placeholder redirect URI — user will paste the URL from their browser.
	redirectURI := "http://127.0.0.1/callback"
	authURL := buildAuthorizeURL(cfg, redirectURI, challenge, state)

	fmt.Fprintf(out, "Open this URL in your browser:\n  %s\n\n", authURL)
	fmt.Fprintf(out, "After authorizing, your browser will redirect to a localhost URL\n")
	fmt.Fprintf(out, "(it may show an error page — that's expected).\n\n")
	fmt.Fprintf(out, "Paste the full URL from your browser's address bar: ")

	line, err := readLine()
	if err != nil {
		return "", fmt.Errorf("could not read URL: %w", err)
	}

	code, err := parseCallbackURL(strings.TrimSpace(line), state)
	if err != nil {
		return "", err
	}

	return exchangeCode(ctx, cfg, code, redirectURI, verifier)
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
			fmt.Fprint(w, htmlPage("Error", "Invalid request. Please try again."))
			return
		}

		// State matches — this is the real callback. Extract the result once.
		code, err := extractCode(q, expectedState)
		once.Do(func() {
			if err != nil {
				resultCh <- callbackResult{Err: err}
			} else {
				resultCh <- callbackResult{Code: code}
			}
		})

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, htmlPage("Error", "Authorization failed. You can close this tab."))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, htmlPage("Success", "Authorization complete! You can close this tab."))
	}
}

func parseCallbackURL(rawURL, expectedState string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	return extractCode(u.Query(), expectedState)
}

// extractCode validates the OAuth callback parameters and returns the authorization code.
// State is checked first to reject forged responses before trusting any other parameters.
func extractCode(q url.Values, expectedState string) (string, error) {
	if q.Get("state") != expectedState {
		return "", fmt.Errorf("state mismatch: possible CSRF attack")
	}

	if errParam := q.Get("error"); errParam != "" {
		desc := q.Get("error_description")
		if desc == "" {
			desc = errParam
		}
		return "", fmt.Errorf("authorization denied: %s", desc)
	}

	code := q.Get("code")
	if code == "" {
		return "", fmt.Errorf("no authorization code received")
	}
	return code, nil
}

func exchangeCode(ctx context.Context, cfg FlowConfig, code, redirectURI, verifier string) (string, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("could not build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("could not read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed (HTTP %d)", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("could not parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response did not contain an access token")
	}

	return tokenResp.AccessToken, nil
}

func htmlPage(title, message string) string {
	t := html.EscapeString(title)
	m := html.EscapeString(message)
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%s — Gumroad CLI</title>
<style>body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f9f9f9}
.card{text-align:center;padding:2rem;border-radius:8px;background:white;box-shadow:0 1px 3px rgba(0,0,0,0.12)}</style>
</head><body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, t, t, m)
}
