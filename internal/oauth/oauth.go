package oauth

import "time"

const (
	// ClientID is the public OAuth application UID for the Gumroad CLI.
	// This is a public client (confidential: false) — the client ID is not secret.
	ClientID = "oljO5HmcOWvCZ5wbitpXPXk3u0LjAb5GdAEBBU5hwKA"

	AuthorizeURL  = "https://app.gumroad.com/oauth/authorize"
	TokenURL      = "https://app.gumroad.com/oauth/token" //nolint:gosec // G101: not a credential
	DeviceCodeURL = "https://app.gumroad.com/oauth/device/code"

	// The public API gates each endpoint on a scope, falling back to the broad
	// legacy `account` scope — but a token authorized for a team store never
	// carries `account`: it keeps only what the requested scopes intersect with
	// the member's role, so login asks for the scopes the command surface needs
	// instead of relying on that fallback. `edit_profile` and `view_profile`
	// additionally satisfy the endpoints that refuse the fallback outright.
	Scopes = "account edit_products edit_profile view_profile view_sales edit_sales mark_sales_as_shipped view_payouts edit_emails"

	DeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	DefaultTimeout = 2 * time.Minute
)
