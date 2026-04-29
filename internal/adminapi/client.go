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

func defaultBaseURL() string {
	if v := strings.TrimRight(os.Getenv(EnvAPIBaseURL), "/"); v != "" {
		return v
	}
	return defaultAPIBaseURL
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
