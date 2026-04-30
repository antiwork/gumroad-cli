package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
)

const (
	EnvAPIBaseURL     = "GUMROAD_DISCOVER_API_BASE_URL"
	defaultAPIBaseURL = "https://gumroad.com"
)

type Client struct {
	api *api.Client
}

func NewClientWithContext(ctx context.Context, version string, debug bool) *Client {
	return NewClientWithBaseURL(ctx, version, debug, defaultBaseURL())
}

func NewClientWithBaseURL(ctx context.Context, version string, debug bool, baseURL string) *Client {
	return &Client{api: api.NewClientWithBaseURL(ctx, "", version, debug, baseURL)}
}

func (c *Client) SetDebugWriter(w io.Writer) {
	if c == nil || c.api == nil {
		return
	}
	c.api.SetDebugWriter(w)
}

func (c *Client) Get(path string, params url.Values) (json.RawMessage, error) {
	data, err := c.api.Get(path, params)
	return data, rewritePublicError(err)
}

func defaultBaseURL() string {
	if v := strings.TrimRight(os.Getenv(EnvAPIBaseURL), "/"); v != "" {
		return v
	}
	return defaultAPIBaseURL
}

func rewritePublicError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	replacement := *apiErr
	switch apiErr.StatusCode {
	case 404:
		replacement.Hint = "Discover endpoint not found — Gumroad may have changed the API."
	case 429:
		replacement.Hint = "Rate limited — slow paginated requests with --page-delay."
	}
	return &replacement
}
