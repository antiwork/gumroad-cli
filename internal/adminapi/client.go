package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/api"
)

const (
	EnvAPIBaseURL      = "GUMROAD_ADMIN_API_BASE_URL"
	defaultAPIBaseURL  = "https://api.gumroad.com"
	adminAPIPathPrefix = "/internal/admin"
)

type Client struct {
	api *api.Client
}

type AdminToken struct {
	Token           string            `json:"token"`
	TokenExternalID string            `json:"token_external_id"`
	Actor           adminconfig.Actor `json:"actor"`
	ExpiresAt       string            `json:"expires_at"`
}

type WhoamiResponse struct {
	Actor  adminconfig.Actor `json:"actor"`
	Token  TokenMetadata     `json:"token"`
	Scopes []string          `json:"scopes"`
}

type TokenMetadata struct {
	ExternalID string `json:"external_id"`
	ExpiresAt  string `json:"expires_at"`
}

func NewClientWithContext(ctx context.Context, token, version string, debug bool) *Client {
	return NewClientWithBaseURL(ctx, token, version, debug, defaultBaseURL())
}

func NewClientWithBaseURL(ctx context.Context, token, version string, debug bool, baseURL string) *Client {
	return &Client{api: api.NewClientWithBaseURL(ctx, token, version, debug, baseURL)}
}

func (c *Client) SetDebugWriter(w io.Writer) {
	if c == nil || c.api == nil {
		return
	}
	c.api.SetDebugWriter(w)
}

func (c *Client) Get(path string, params url.Values) (json.RawMessage, error) {
	data, err := c.api.Get(AdminPath(path), params)
	return data, rewriteAdminError(err)
}

func (c *Client) Post(path string, params url.Values) (json.RawMessage, error) {
	data, err := c.api.Post(AdminPath(path), params)
	return data, rewriteAdminError(err)
}

func (c *Client) PostJSON(path string, payload any) (json.RawMessage, error) {
	data, err := c.api.PostJSON(AdminPath(path), payload)
	return data, rewriteAdminError(err)
}

func (c *Client) Put(path string, params url.Values) (json.RawMessage, error) {
	data, err := c.api.Put(AdminPath(path), params)
	return data, rewriteAdminError(err)
}

func (c *Client) PutJSON(path string, payload any) (json.RawMessage, error) {
	data, err := c.api.PutJSON(AdminPath(path), payload)
	return data, rewriteAdminError(err)
}

func (c *Client) Delete(path string, params url.Values) (json.RawMessage, error) {
	data, err := c.api.Delete(AdminPath(path), params)
	return data, rewriteAdminError(err)
}

func (c *Client) Whoami() (WhoamiResponse, error) {
	data, err := c.Get("/whoami", url.Values{})
	if err != nil {
		return WhoamiResponse{}, err
	}
	var resp WhoamiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return WhoamiResponse{}, err
	}
	return resp, nil
}

func (c *Client) RevokeSelf() error {
	_, err := c.PostJSON("/auth/revoke", struct{}{})
	return err
}

func ExchangeAuthorizationCode(ctx context.Context, code, codeVerifier, version string, debug bool) (AdminToken, error) {
	client := NewClientWithContext(ctx, "", version, debug)
	data, err := client.PostJSON("/auth/exchange", map[string]string{
		"code":          code,
		"code_verifier": codeVerifier,
	})
	if err != nil {
		return AdminToken{}, err
	}
	var token AdminToken
	if err := json.Unmarshal(data, &token); err != nil {
		return AdminToken{}, err
	}
	return token, nil
}

func defaultBaseURL() string {
	if v := strings.TrimRight(os.Getenv(EnvAPIBaseURL), "/"); v != "" {
		return v
	}
	return defaultAPIBaseURL
}

func AdminTokensURL() string {
	base := defaultBaseURL()
	if strings.Contains(base, "://api.gumroad.com") {
		base = strings.Replace(base, "://api.gumroad.com", "://app.gumroad.com", 1)
	}
	return strings.TrimRight(base, "/") + "/admin/cli/tokens"
}

// AdminPath returns the absolute admin-prefixed path for path.
func AdminPath(path string) string {
	if path == "" || path == "/" {
		return adminAPIPathPrefix
	}
	if strings.HasPrefix(path, "/") {
		return adminAPIPathPrefix + path
	}
	return adminAPIPathPrefix + "/" + path
}

func rewriteAdminError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	replacement := *apiErr
	switch apiErr.StatusCode {
	case 401:
		replacement.Hint = adminconfig.HintSetAdminToken
	case 403:
		replacement.Hint = "Check that your admin token has internal admin access."
	}
	return &replacement
}
