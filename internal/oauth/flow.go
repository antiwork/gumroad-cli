package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
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
	DeviceCodeURL string
	Scopes        string
	OptionalAdmin bool
	Timeout       time.Duration
	HTTPClient    *http.Client // optional; defaults to http.DefaultClient
	Sleep         func(context.Context, time.Duration) error
	DebugWriter   io.Writer // optional; when set, poll attempts and outcomes are logged
	StatusWriter  io.Writer // optional; when set, retry waits and throttled polls are reported to the user

	now func() time.Time
}

// tokenRequestTimeout caps how long a single token endpoint request can hang.
// Without it, a dead reused connection stalls a request for the full flow
// timeout (2 minutes) with no feedback before the next attempt.
const tokenRequestTimeout = 30 * time.Second

// currentTime reads the wall clock, without Go's monotonic reading, so a
// laptop that slept through a retry wait still sees the code's real age.
func (cfg FlowConfig) currentTime() time.Time {
	if cfg.now != nil {
		return cfg.now()
	}
	return time.Now().Round(0)
}

func (cfg FlowConfig) statusf(format string, args ...any) {
	if cfg.StatusWriter == nil {
		return
	}
	fmt.Fprintf(cfg.StatusWriter, format+"\n", args...)
}

func (cfg FlowConfig) debugf(format string, args ...any) {
	if cfg.DebugWriter == nil {
		return
	}
	fmt.Fprintf(cfg.DebugWriter, "DEBUG "+format+"\n", args...)
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
		DeviceCodeURL: DeviceCodeURL,
		Scopes:        Scopes,
		OptionalAdmin: true,
		Timeout:       DefaultTimeout,
	}
}

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Interval         int    `json:"interval"`
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

	authCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
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
	case <-authCtx.Done():
		if authCtx.Err() != context.DeadlineExceeded {
			return FlowResult{}, fmt.Errorf("authorization cancelled")
		}
		// Drain resultCh in case the callback arrived at the exact deadline.
		select {
		case result = <-resultCh:
		default:
			return FlowResult{}, fmt.Errorf("authorization timed out after %s", cfg.Timeout)
		}
	}
	if result.Err != nil {
		return FlowResult{}, result.Err
	}

	// The timeout covers waiting for the seller to authorize. The exchange
	// that follows is limited by its own retry budget and per-request cap, so
	// a seller who used most of the wait still gets the full retry.
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

	readCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
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
	case <-readCtx.Done():
		return FlowResult{}, fmt.Errorf("authorization timed out after %s", cfg.Timeout)
	}

	payload, err := parseCallbackPayload(strings.TrimSpace(line), state)
	if err != nil {
		return FlowResult{}, err
	}

	return exchangeCodeResult(ctx, cfg, payload, redirectURI, verifier)
}

func DeviceFlowResult(ctx context.Context, cfg FlowConfig, out io.Writer) (FlowResult, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}

	deviceCode, err := requestDeviceCode(ctx, cfg)
	if err != nil {
		return FlowResult{}, err
	}
	cfg.debugf("device code request url=%s expires_in=%d interval=%d", cfg.DeviceCodeURL, deviceCode.ExpiresIn, deviceCode.Interval)
	if deviceCode.DeviceCode == "" {
		return FlowResult{}, fmt.Errorf("device code response did not contain a device code")
	}
	if deviceCode.ExpiresIn <= 0 {
		return FlowResult{}, fmt.Errorf("device code response did not contain a valid expiration")
	}

	writeDeviceInstructions(out, deviceCode)
	return pollDeviceToken(ctx, cfg, deviceCode)
}

func requestDeviceCode(ctx context.Context, cfg FlowConfig) (DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": {cfg.ClientID},
		"scope":     {cfg.Scopes},
	}
	if cfg.OptionalAdmin {
		data.Set("admin_scope", "optional")
	}

	requestCtx, cancel := requestContext(ctx, cfg)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, "POST", cfg.DeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("could not build device authorization request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("device authorization failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("could not read device authorization response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return DeviceCodeResponse{}, oauthHTTPError("device authorization failed", resp.StatusCode, body)
	}

	var deviceCode DeviceCodeResponse
	if err := json.Unmarshal(body, &deviceCode); err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("could not parse device authorization response: %w", err)
	}
	return deviceCode, nil
}

func writeDeviceInstructions(out io.Writer, deviceCode DeviceCodeResponse) {
	if out == nil {
		return
	}

	verificationURL := strings.TrimSpace(deviceCode.VerificationURIComplete)
	if verificationURL == "" {
		verificationURL = strings.TrimSpace(deviceCode.VerificationURI)
	}

	fmt.Fprintf(out, "Open this URL to authorize Gumroad CLI:\n  %s\n\n", verificationURL)
	if strings.TrimSpace(deviceCode.UserCode) != "" {
		fmt.Fprintf(out, "Code: %s\n\n", deviceCode.UserCode)
	}
	fmt.Fprintln(out, "Waiting for approval...")
}

func pollDeviceToken(ctx context.Context, cfg FlowConfig, deviceCode DeviceCodeResponse) (FlowResult, error) {
	interval := time.Duration(deviceCode.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresAt := cfg.currentTime().Add(time.Duration(deviceCode.ExpiresIn) * time.Second)

	poll := 0
	nextWait := time.Duration(0)
	throttled := false
	inThrottle := false
	for {
		remaining := expiresAt.Sub(cfg.currentTime())
		if remaining <= 0 {
			return FlowResult{}, deviceExpiredError(throttled)
		}
		wait := interval
		if nextWait > 0 {
			// A throttle's Retry-After governs the next attempt only: as a
			// standing interval it would delay an approval that arrives
			// after the throttle cleared.
			wait = nextWait
			nextWait = 0
		}
		if wait > remaining {
			wait = remaining
		}
		if err := cfg.Sleep(ctx, wait); err != nil {
			return FlowResult{}, fmt.Errorf("authorization cancelled: %w", err)
		}

		poll++
		result, nextInterval, err := pollDeviceTokenOnce(ctx, cfg, deviceCode.DeviceCode, interval)
		if err == nil {
			cfg.debugf("device poll attempt=%d outcome=approved", poll)
			return result, nil
		}
		var oauthErr *oauthDevicePollError
		if !errors.As(err, &oauthErr) {
			var transient *transientPollError
			if !errors.As(err, &transient) {
				// A completed response we could not use (invalid JSON,
				// missing access token, unexpected HTTP status) is a
				// permanent contract failure; retrying would hide it.
				return FlowResult{}, err
			}
			// Transport failures and server refusals both leave the
			// authorization window open server-side, so keep polling
			// until it expires instead of aborting the whole login.
			if ctx.Err() != nil {
				return FlowResult{}, fmt.Errorf("authorization cancelled: %w", err)
			}
			nextWait = transient.retryAfter
			cfg.debugf("device poll attempt=%d outcome=transient_error retrying retry_after=%s err=%q", poll, transient.retryAfter, err)
			if transient.status == http.StatusTooManyRequests {
				throttled = true
				if !inThrottle {
					inThrottle = true
					cfg.statusf("Gumroad is rate limiting login requests from this network address; still waiting for approval and retrying in %s...", nextDevicePollWait(nextWait, interval, expiresAt.Sub(cfg.currentTime())).Round(time.Second))
				}
			} else if transient.status != 0 {
				inThrottle = false
			}
			continue
		}
		inThrottle = false
		cfg.debugf("device poll attempt=%d outcome=%s", poll, oauthErr.Code)
		switch oauthErr.Code {
		case "authorization_pending":
			continue
		case "slow_down":
			interval = nextInterval
			continue
		case "access_denied":
			return FlowResult{}, fmt.Errorf("authorization denied: access was denied")
		case "expired_token":
			return FlowResult{}, deviceExpiredError(throttled)
		default:
			return FlowResult{}, oauthErr
		}
	}
}

func nextDevicePollWait(retryAfter, interval, remaining time.Duration) time.Duration {
	wait := interval
	if retryAfter > 0 {
		wait = retryAfter
	}
	return max(min(wait, remaining), 0)
}

// ErrDeviceLoginRateLimited is returned when the device code expired after
// Gumroad throttled at least one poll, so an approval may have happened but
// could not be collected. The browser flow shares the same limit, so callers
// should not suggest it as an alternative.
var ErrDeviceLoginRateLimited error = deviceRateLimitedError{}

type deviceRateLimitedError struct{}

func (deviceRateLimitedError) Error() string {
	return "authorization expired: Gumroad rate limited the login polls from this network address, so an approval may not have reached the CLI before the code expired. Nothing was saved. Wait before running `gumroad auth login` again, and run one login at a time"
}

func deviceExpiredError(throttled bool) error {
	if throttled {
		return ErrDeviceLoginRateLimited
	}
	return errors.New("authorization expired")
}

type oauthDevicePollError struct {
	Code        string
	Description string
}

func (e *oauthDevicePollError) Error() string {
	if e.Description == "" || e.Description == e.Code {
		return e.Code
	}
	return e.Code + ": " + e.Description
}

// transientPollError marks a poll failure that left the pending authorization
// intact: the request never completed, or the endpoint answered 429 or 5xx.
// Retrying is safe for those; a response we could not use (bad JSON, missing
// token) is a permanent contract failure.
type transientPollError struct {
	err        error
	retryAfter time.Duration
	status     int
}

func (e *transientPollError) Error() string { return e.err.Error() }

func (e *transientPollError) Unwrap() error { return e.err }

func pollDeviceTokenOnce(ctx context.Context, cfg FlowConfig, deviceCode string, currentInterval time.Duration) (FlowResult, time.Duration, error) {
	data := url.Values{
		"grant_type":  {DeviceGrantType},
		"client_id":   {cfg.ClientID},
		"device_code": {deviceCode},
	}

	requestCtx, cancel := tokenRequestContext(ctx, cfg)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, "POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return FlowResult{}, currentInterval, fmt.Errorf("could not build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return FlowResult{}, currentInterval, &transientPollError{err: fmt.Errorf("token exchange failed: %w", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		if resp.StatusCode == http.StatusOK {
			// A 200 means the server approved the login and may have already
			// consumed the device code while issuing the token. Polling again
			// with the same code can then fail with expired_token or
			// invalid_grant even though the user approved.
			//
			// The read can fail after the full body already arrived (for
			// example when the declared Content-Length overshoots the actual
			// body). If the bytes we did receive form a valid token response,
			// finish the login with them instead of throwing away a completed
			// approval. Otherwise surface the failure with a clear next step
			// instead of retrying.
			if len(body) > 0 {
				if result, parseErr := tokenResponseResult(body, "", ""); parseErr == nil {
					return result, currentInterval, nil
				}
			}
			return FlowResult{}, currentInterval, fmt.Errorf("connection failed while receiving the approved token; run the login command again: %w", err)
		}
		return FlowResult{}, currentInterval, &transientPollError{err: fmt.Errorf("could not read token response: %w", err), retryAfter: retryAfterWait(resp), status: resp.StatusCode}
	}

	if resp.StatusCode == http.StatusOK {
		result, err := tokenResponseResult(body, "", "")
		return result, currentInterval, err
	}

	oauthErr := parseOAuthError(body)
	if oauthErr.Error == "slow_down" {
		if oauthErr.Interval > 0 {
			return FlowResult{}, time.Duration(oauthErr.Interval) * time.Second, &oauthDevicePollError{Code: oauthErr.Error, Description: oauthErr.ErrorDescription}
		}
		return FlowResult{}, currentInterval + 5*time.Second, &oauthDevicePollError{Code: oauthErr.Error, Description: oauthErr.ErrorDescription}
	}
	if oauthErr.Error != "" {
		return FlowResult{}, currentInterval, &oauthDevicePollError{Code: oauthErr.Error, Description: oauthErr.ErrorDescription}
	}
	statusErr := fmt.Errorf("token exchange failed (HTTP %d)", resp.StatusCode)
	if !isRetryableTokenStatus(resp.StatusCode) {
		return FlowResult{}, currentInterval, statusErr
	}
	// Retrying is safe: the response carries no OAuth error, and a throttle or
	// a 5xx leaves the pending authorization untouched.
	return FlowResult{}, currentInterval, &transientPollError{err: statusErr, retryAfter: retryAfterWait(resp), status: resp.StatusCode}
}

// isRetryableTokenStatus reports whether a token endpoint status is one that
// clears on its own: the same throttles and server faults the API client
// retries for normal requests.
func isRetryableTokenStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryAfterWait(resp *http.Response) time.Duration {
	delay, _ := api.ParseRetryAfter(resp.Header.Get("Retry-After"))
	return delay
}

func requestContext(ctx context.Context, cfg FlowConfig) (context.Context, context.CancelFunc) {
	if cfg.Timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, cfg.Timeout)
}

// tokenRequestContext bounds a single token endpoint request. Device polls and
// code exchange retries both repeat, so a request hung on a dead reused
// connection should fail fast and let the loop retry on a fresh attempt rather
// than stall for the full flow timeout.
func tokenRequestContext(ctx context.Context, cfg FlowConfig) (context.Context, context.CancelFunc) {
	timeout := tokenRequestTimeout
	if cfg.Timeout > 0 && cfg.Timeout < timeout {
		timeout = cfg.Timeout
	}
	return context.WithTimeout(ctx, timeout)
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

// exchangeRetryWindow bounds how long a login keeps retrying a throttled token
// exchange. The server issues authorization codes that expire after 10 minutes
// (Doorkeeper's authorization_code_expires_in in antiwork/gumroad); stopping a
// minute short keeps a retry from spending the seller's wait on an expired code.
const exchangeRetryWindow = 9 * time.Minute

// maxExchangeAttempts caps the requests a throttled login sends, because each
// retry counts against the same rate limit that throttled it.
const maxExchangeAttempts = 6

// exchangeRetryBackoff is the first wait after a retryable exchange that named
// no wait of its own; each further attempt doubles it.
const exchangeRetryBackoff = time.Second

// transientExchangeError marks an exchange that failed without the app
// spending the authorization code: the request never arrived (a transport
// failure or an edge throttle), or the endpoint answered with a server fault.
type transientExchangeError struct {
	err        error
	retryAfter time.Duration
	status     int
}

func (e *transientExchangeError) Error() string { return e.err.Error() }

func (e *transientExchangeError) Unwrap() error { return e.err }

// exchangeCodeResult exchanges the authorization code for a token, retrying an
// attempt the endpoint refused transiently while the budget lasts. The code is
// single-use, so a retry happens only when nothing suggests the app consumed
// it.
func exchangeCodeResult(ctx context.Context, cfg FlowConfig, payload callbackPayload, redirectURI, verifier string) (FlowResult, error) {
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}

	giveUpAt := cfg.currentTime().Add(exchangeRetryWindow)
	for attempt := 1; ; attempt++ {
		result, err := exchangeCodeOnce(ctx, cfg, payload, redirectURI, verifier)
		if err == nil {
			return result, nil
		}

		var transient *transientExchangeError
		if !errors.As(err, &transient) {
			return FlowResult{}, err
		}

		wait := transient.retryAfter
		if wait <= 0 {
			wait = exchangeBackoff(attempt)
		}
		if attempt >= maxExchangeAttempts || wait >= giveUpAt.Sub(cfg.currentTime()) {
			return FlowResult{}, exchangeRefusedError(transient)
		}
		cfg.debugf("token exchange attempt=%d outcome=retryable wait=%s err=%q", attempt, wait, err)
		if transient.status == http.StatusTooManyRequests {
			cfg.statusf("Gumroad is rate limiting login requests; retrying in %s...", wait.Round(time.Second))
		} else {
			cfg.statusf("Login request failed; retrying in %s...", wait.Round(time.Second))
		}

		if sleepErr := cfg.Sleep(ctx, wait); sleepErr != nil {
			if errors.Is(sleepErr, context.DeadlineExceeded) {
				return FlowResult{}, exchangeRefusedError(transient)
			}
			return FlowResult{}, fmt.Errorf("token exchange cancelled: %w", sleepErr)
		}
		if !cfg.currentTime().Before(giveUpAt) {
			return FlowResult{}, exchangeRefusedError(transient)
		}
	}
}

func exchangeBackoff(attempt int) time.Duration {
	wait := exchangeRetryBackoff
	for i := 1; i < attempt; i++ {
		wait *= 2
	}
	return wait
}

// exchangeRefusedError explains a refusal the login could not wait out. A
// throttle is not about the seller's account and nothing was stored, so the
// remedy is the wait itself rather than another authorization.
func exchangeRefusedError(transient *transientExchangeError) error {
	if transient.status != http.StatusTooManyRequests {
		return transient.err
	}
	return fmt.Errorf("%w: Gumroad is rate limiting logins from this network address, so the login could not finish. Nothing was saved, and retrying does not shorten the wait — run `gumroad auth login --web` again later", transient.err)
}

func exchangeCodeOnce(ctx context.Context, cfg FlowConfig, payload callbackPayload, redirectURI, verifier string) (FlowResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {payload.Code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}

	requestCtx, cancel := tokenRequestContext(ctx, cfg)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, "POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return FlowResult{}, fmt.Errorf("could not build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return FlowResult{}, &transientExchangeError{err: fmt.Errorf("token exchange failed: %w", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		// A 200 means the app already issued the token and spent the code, so
		// that read failure stays permanent; a throttled or failed response
		// left the code usable and still carries Retry-After.
		if parseOAuthError(body).Error != "" {
			return FlowResult{}, oauthHTTPError("token exchange failed", resp.StatusCode, body)
		}
		if isRetryableTokenStatus(resp.StatusCode) {
			return FlowResult{}, &transientExchangeError{err: fmt.Errorf("could not read token response: %w", err), retryAfter: retryAfterWait(resp), status: resp.StatusCode}
		}
		return FlowResult{}, fmt.Errorf("could not read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		statusErr := oauthHTTPError("token exchange failed", resp.StatusCode, body)
		if parseOAuthError(body).Error != "" || !isRetryableTokenStatus(resp.StatusCode) {
			return FlowResult{}, statusErr
		}
		// Retrying is safe: the response carries no OAuth error, and a throttle
		// or a server fault leaves the authorization code unspent.
		return FlowResult{}, &transientExchangeError{err: statusErr, retryAfter: retryAfterWait(resp), status: resp.StatusCode}
	}

	return tokenResponseResult(body, payload.AdminCode, verifier)
}

func tokenResponseResult(body []byte, adminCode, verifier string) (FlowResult, error) {
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
		AdminAuthorizationCode: adminCode,
		CodeVerifier:           verifier,
	}, nil
}

func oauthHTTPError(prefix string, statusCode int, body []byte) error {
	oauthErr := parseOAuthError(body)
	if oauthErr.Error == "" {
		return fmt.Errorf("%s (HTTP %d)", prefix, statusCode)
	}
	desc := oauthErr.ErrorDescription
	if desc == "" || desc == oauthErr.Error {
		return fmt.Errorf("%s: %s (HTTP %d)", prefix, oauthErr.Error, statusCode)
	}
	return fmt.Errorf("%s: %s: %s (HTTP %d)", prefix, oauthErr.Error, desc, statusCode)
}

func parseOAuthError(body []byte) oauthErrorResponse {
	var oauthErr oauthErrorResponse
	_ = json.Unmarshal(body, &oauthErr)
	return oauthErr
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
