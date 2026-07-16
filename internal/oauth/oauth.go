package oauth

import "time"

const (
	// ClientID is the public OAuth application UID for the Gumroad CLI.
	// This is a public client (confidential: false) — the client ID is not secret.
	ClientID = "oljO5HmcOWvCZ5wbitpXPXk3u0LjAb5GdAEBBU5hwKA"

	AuthorizeURL  = "https://app.gumroad.com/oauth/authorize"
	TokenURL      = "https://app.gumroad.com/oauth/token" //nolint:gosec // G101: not a credential
	DeviceCodeURL = "https://app.gumroad.com/oauth/device/code"

	// The broad legacy `account` scope covers most of the public API, but the
	// storefront pages write endpoints additionally require the narrower
	// `edit_profile` scope on the token itself (they deliberately refuse the
	// `account` fallback), so login requests both.
	Scopes = "account edit_profile"

	DeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	DefaultTimeout = 2 * time.Minute
)
