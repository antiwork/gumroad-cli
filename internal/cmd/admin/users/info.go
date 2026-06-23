package users

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type infoResponse struct {
	UserID string   `json:"user_id"`
	User   userInfo `json:"user"`
}

type userInfo struct {
	Email                          string            `json:"email"`
	Name                           string            `json:"name"`
	Username                       string            `json:"username"`
	ProfileURL                     string            `json:"profile_url"`
	Country                        string            `json:"country"`
	Locale                         string            `json:"locale"`
	Timezone                       string            `json:"timezone"`
	CreatedAt                      string            `json:"created_at"`
	DeletedAt                      string            `json:"deleted_at"`
	RiskState                      riskState         `json:"risk_state"`
	ActiveWatchedUser              *watchedUserInfo  `json:"active_watched_user"`
	TwoFactorAuthenticationEnabled bool              `json:"two_factor_authentication_enabled"`
	SignIn                         signInInfo        `json:"sign_in"`
	Social                         socialInfo        `json:"social"`
	Payouts                        payoutsInfo       `json:"payouts"`
	Stats                          statsInfo         `json:"stats"`
	Stripe                         *stripeInfo       `json:"stripe"`
	AdminLinks                     map[string]string `json:"admin_links"`
}

type signInInfo struct {
	AccountCreatedIP string `json:"account_created_ip"`
	CurrentIP        string `json:"current_ip"`
	CurrentAt        string `json:"current_at"`
	LastIP           string `json:"last_ip"`
	LastAt           string `json:"last_at"`
	Count            int    `json:"count"`
}

type socialInfo struct {
	TwitterUserID           string                       `json:"twitter_user_id"`
	TwitterHandle           string                       `json:"twitter_handle"`
	FacebookUID             string                       `json:"facebook_uid"`
	GoogleUID               string                       `json:"google_uid"`
	OAuthProvider           string                       `json:"oauth_provider"`
	ExternalAuthentications []externalAuthenticationInfo `json:"external_authentications"`
}

type externalAuthenticationInfo struct {
	Provider string `json:"provider"`
	UID      string `json:"uid"`
	LinkedAt string `json:"linked_at"`
}

type riskState struct {
	Status                 string `json:"status"`
	UserRiskState          string `json:"user_risk_state"`
	Suspended              bool   `json:"suspended"`
	FlaggedForFraud        bool   `json:"flagged_for_fraud"`
	FlaggedForTOSViolation bool   `json:"flagged_for_tos_violation"`
	OnProbation            bool   `json:"on_probation"`
	LastStatusChangedAt    string `json:"last_status_changed_at"`
}

type payoutsInfo struct {
	PausedInternally     bool   `json:"paused_internally"`
	PausedByUser         bool   `json:"paused_by_user"`
	PausedBySource       string `json:"paused_by_source"`
	PausedForReason      string `json:"paused_for_reason"`
	NextPayoutDate       string `json:"next_payout_date"`
	BalanceForNextPayout string `json:"balance_for_next_payout"`
}

type statsInfo struct {
	SalesCount             int    `json:"sales_count"`
	TotalEarningsFormatted string `json:"total_earnings_formatted"`
	UnpaidBalanceFormatted string `json:"unpaid_balance_formatted"`
	CommentsCount          int    `json:"comments_count"`
}

type stripeInfo struct {
	Connected              bool                `json:"connected"`
	StripeConnectAccountID string              `json:"stripe_connect_account_id"`
	StripeDashboardURL     string              `json:"stripe_dashboard_url"`
	Verification           *stripeVerification `json:"verification"`
}

type stripeVerification struct {
	Error                    string `json:"error"`
	ChargesEnabled           bool   `json:"charges_enabled"`
	PayoutsEnabled           bool   `json:"payouts_enabled"`
	DetailsSubmitted         bool   `json:"details_submitted"`
	DisabledReason           string `json:"disabled_reason"`
	CurrentlyDueCount        int    `json:"currently_due_count"`
	PastDueCount             int    `json:"past_due_count"`
	PendingVerificationCount int    `json:"pending_verification_count"`
}

func newInfoCmd() *cobra.Command {
	var lookup userLookupFlags

	cmd := &cobra.Command{
		Use:   "info",
		Short: "View a comprehensive admin info payload for a user",
		Long: `View a comprehensive admin info payload for a user, combining identity,
risk state, two-factor state, payouts state, earnings/sales stats, Stripe
Connect verification, and admin deep-links into a single response. Mirrors the
admin web user-detail page so support workflows can resolve from a single CLI
invocation without round-tripping to Stripe or the admin dashboard.

The Stripe section reports whether a Stripe Connect account is attached and,
when it is, its account id, dashboard URL, and live verification flags
(charges/payouts enabled, details submitted) plus any outstanding requirement
counts. If the live Stripe lookup fails it degrades to a verification error
line instead. Admin links carry impersonate, user, purchases, and (when a
Stripe account exists) Stripe-dashboard helper URLs.

Identify the user with --email, --user-id, or --username. When more than one
is supplied, the server resolves by --user-id first, then --email, then
--username.`,
		Example: `  gumroad admin users info --email user@example.com
  gumroad admin users info --user-id 2245593582708
  gumroad admin users info --username sellerone
  gumroad admin users info --email user@example.com --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}

			identifier := target.Identifier()
			return admincmd.RunGetDecoded[infoResponse](opts, "Fetching user info...", "/users/info", target.Values(), func(resp infoResponse) error {
				return renderInfo(opts, identifier, resp.UserID, resp.User)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)

	return cmd
}

func renderInfo(opts cmdutil.Options, identifier, userID string, info userInfo) error {
	if opts.PlainOutput {
		stripeConnected, stripeAccountID := "", ""
		if info.Stripe != nil {
			stripeConnected = strconv.FormatBool(info.Stripe.Connected)
			stripeAccountID = info.Stripe.StripeConnectAccountID
		}
		return output.PrintPlain(opts.Out(), [][]string{{
			fallback(info.Email, identifier),
			info.Name,
			info.Username,
			fallback(info.RiskState.Status, info.RiskState.UserRiskState),
			strconv.FormatBool(info.TwoFactorAuthenticationEnabled),
			strconv.FormatBool(info.Payouts.PausedInternally),
			strconv.FormatBool(info.Payouts.PausedByUser),
			info.Payouts.NextPayoutDate,
			strconv.Itoa(info.Stats.SalesCount),
			info.Stats.TotalEarningsFormatted,
			info.CreatedAt,
			stripeConnected,
			stripeAccountID,
		}})
	}

	if opts.Quiet {
		return nil
	}

	var b strings.Builder
	style := opts.Style()

	headline := info.Name
	if headline == "" {
		headline = fallback(info.Email, identifier)
	}
	fmt.Fprintln(&b, style.Bold(headline))
	if info.Email != headline {
		writeOptional(&b, "Email", info.Email)
	}
	if userID != headline {
		writeOptional(&b, "User ID", userID)
	}
	writeOptional(&b, "Username", info.Username)
	writeOptional(&b, "Profile", info.ProfileURL)
	writeOptional(&b, "Country", info.Country)
	writeOptional(&b, "Locale", info.Locale)
	writeOptional(&b, "Timezone", info.Timezone)
	writeOptional(&b, "Created", info.CreatedAt)
	writeOptional(&b, "Deleted", info.DeletedAt)

	fmt.Fprintln(&b)
	writeRiskState(&b, info.RiskState)

	twoFactor := "disabled"
	if info.TwoFactorAuthenticationEnabled {
		twoFactor = "enabled"
	}
	fmt.Fprintf(&b, "Two-factor: %s\n", twoFactor)

	if !info.SignIn.empty() {
		fmt.Fprintln(&b)
		writeSignIn(&b, info.SignIn)
	}

	if !info.Social.empty() {
		fmt.Fprintln(&b)
		if err := writeSocial(&b, style, info.Social); err != nil {
			return err
		}
	}

	if info.ActiveWatchedUser != nil {
		fmt.Fprintln(&b)
		writeWatchlist(&b, info.ActiveWatchedUser)
	}

	fmt.Fprintln(&b)
	writePayouts(&b, info.Payouts)

	if info.Stripe != nil {
		fmt.Fprintln(&b)
		writeStripe(&b, *info.Stripe)
	}

	fmt.Fprintln(&b)
	writeStats(&b, info.Stats)

	if len(info.AdminLinks) > 0 {
		fmt.Fprintln(&b)
		writeAdminLinks(&b, info.AdminLinks)
	}

	return output.Writef(opts.Out(), "%s", b.String())
}

func writeOptional(b *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(b, "%s: %s\n", label, value)
	}
}

func writeRiskState(b *strings.Builder, risk riskState) {
	status := risk.Status
	if status == "" {
		status = risk.UserRiskState
	}
	if status != "" {
		fmt.Fprintf(b, "Risk: %s\n", status)
	}
	if risk.UserRiskState != "" && risk.UserRiskState != status {
		fmt.Fprintf(b, "  user_risk_state: %s\n", risk.UserRiskState)
	}
	for _, flag := range []struct {
		name string
		on   bool
	}{
		{"suspended", risk.Suspended},
		{"flagged_for_fraud", risk.FlaggedForFraud},
		{"flagged_for_tos_violation", risk.FlaggedForTOSViolation},
		{"on_probation", risk.OnProbation},
	} {
		if flag.on {
			fmt.Fprintf(b, "  %s: true\n", flag.name)
		}
	}
	if risk.LastStatusChangedAt != "" {
		fmt.Fprintf(b, "  last status change: %s\n", risk.LastStatusChangedAt)
	}
}

func (s signInInfo) empty() bool {
	return s.AccountCreatedIP == "" &&
		s.CurrentIP == "" &&
		s.CurrentAt == "" &&
		s.LastIP == "" &&
		s.LastAt == "" &&
		s.Count == 0
}

func writeSignIn(b *strings.Builder, signIn signInInfo) {
	fmt.Fprintln(b, "Sign-in:")
	if signIn.AccountCreatedIP != "" {
		fmt.Fprintf(b, "  account-created IP: %s\n", signIn.AccountCreatedIP)
	}
	if current := formatIPWithTime(signIn.CurrentIP, signIn.CurrentAt); current != "" {
		fmt.Fprintf(b, "  current: %s\n", current)
	}
	if last := formatIPWithTime(signIn.LastIP, signIn.LastAt); last != "" {
		fmt.Fprintf(b, "  last: %s\n", last)
	}
	if signIn.Count > 0 {
		fmt.Fprintf(b, "  count: %d\n", signIn.Count)
	}
}

func formatIPWithTime(ip, at string) string {
	switch {
	case ip != "" && at != "":
		return ip + " at " + at
	case ip != "":
		return ip
	default:
		return at
	}
}

func (s socialInfo) empty() bool {
	return s.TwitterUserID == "" &&
		s.TwitterHandle == "" &&
		s.FacebookUID == "" &&
		s.GoogleUID == "" &&
		s.OAuthProvider == "" &&
		!hasExternalAuthentications(s.ExternalAuthentications)
}

func hasExternalAuthentications(auths []externalAuthenticationInfo) bool {
	for _, auth := range auths {
		if auth.Provider != "" || auth.UID != "" || auth.LinkedAt != "" {
			return true
		}
	}
	return false
}

func writeSocial(b *strings.Builder, style output.Styler, social socialInfo) error {
	fmt.Fprintln(b, "Social:")
	if twitter := formatTwitter(social.TwitterHandle, social.TwitterUserID); twitter != "" {
		fmt.Fprintf(b, "  twitter: %s\n", twitter)
	}
	writeOptionalIndented(b, "facebook UID", social.FacebookUID)
	writeOptionalIndented(b, "google UID", social.GoogleUID)
	writeOptionalIndented(b, "OAuth provider", social.OAuthProvider)

	if hasExternalAuthentications(social.ExternalAuthentications) {
		fmt.Fprintln(b, "  external authentications:")
		tbl := output.NewStyledTable(style, "PROVIDER", "UID", "LINKED")
		for _, auth := range social.ExternalAuthentications {
			if auth.Provider == "" && auth.UID == "" && auth.LinkedAt == "" {
				continue
			}
			tbl.AddRow(auth.Provider, auth.UID, auth.LinkedAt)
		}
		if err := writeIndentedTable(b, tbl, "    "); err != nil {
			return err
		}
	}
	return nil
}

func formatTwitter(handle, userID string) string {
	handle = strings.TrimPrefix(handle, "@")
	switch {
	case handle != "" && userID != "":
		return "@" + handle + " (id: " + userID + ")"
	case handle != "":
		return "@" + handle
	case userID != "":
		return "(id: " + userID + ")"
	default:
		return ""
	}
}

func writeOptionalIndented(b *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(b, "  %s: %s\n", label, value)
	}
}

func writeIndentedTable(b *strings.Builder, tbl *output.Table, indent string) error {
	var table strings.Builder
	if err := tbl.Render(&table); err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimRight(table.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		fmt.Fprintf(b, "%s%s\n", indent, line)
	}
	return nil
}

func writeWatchlist(b *strings.Builder, watchedUser *watchedUserInfo) {
	fmt.Fprintln(b, "Watchlist: active")
	if watchedUser.ID != "" {
		fmt.Fprintf(b, "  id: %s\n", watchedUser.ID)
	}
	if watchedUser.RevenueThresholdCents > 0 {
		fmt.Fprintf(b, "  revenue: %s of %s\n", formatWatchMoney(watchedUser.RevenueCents), formatWatchMoney(watchedUser.RevenueThresholdCents))
	}
	if watchedUser.UnpaidBalanceCents > 0 {
		fmt.Fprintf(b, "  unpaid balance: %s\n", formatWatchMoney(watchedUser.UnpaidBalanceCents))
	}
	if watchedUser.Notes != "" {
		fmt.Fprintf(b, "  note: %s\n", watchedUser.Notes)
	}
	if watchedUser.CreatedAt != "" {
		fmt.Fprintf(b, "  created: %s\n", watchedUser.CreatedAt)
	}
	if watchedUser.LastSyncedAt != "" {
		fmt.Fprintf(b, "  last synced: %s\n", watchedUser.LastSyncedAt)
	}
}

func writePayouts(b *strings.Builder, p payoutsInfo) {
	fmt.Fprintf(b, "Payouts: %s\n", payoutsHeadlineState(p))
	if p.PausedBySource != "" {
		fmt.Fprintf(b, "  paused by: %s\n", p.PausedBySource)
	}
	if p.PausedForReason != "" {
		fmt.Fprintf(b, "  reason: %s\n", p.PausedForReason)
	}
	if p.NextPayoutDate != "" {
		fmt.Fprintf(b, "  next payout: %s\n", p.NextPayoutDate)
	}
	if p.BalanceForNextPayout != "" {
		fmt.Fprintf(b, "  balance for next payout: %s\n", p.BalanceForNextPayout)
	}
}

func payoutsHeadlineState(p payoutsInfo) string {
	switch {
	case p.PausedInternally:
		return "paused (internal)"
	case p.PausedByUser:
		return "paused (by user)"
	default:
		return "active"
	}
}

func writeStripe(b *strings.Builder, s stripeInfo) {
	if !s.Connected {
		fmt.Fprintln(b, "Stripe: not connected")
		return
	}

	fmt.Fprintln(b, "Stripe: connected")
	if s.StripeConnectAccountID != "" {
		fmt.Fprintf(b, "  account: %s\n", s.StripeConnectAccountID)
	}
	if s.StripeDashboardURL != "" {
		fmt.Fprintf(b, "  dashboard: %s\n", s.StripeDashboardURL)
	}
	if s.Verification == nil {
		return
	}
	if s.Verification.Error != "" {
		fmt.Fprintf(b, "  verification error: %s\n", s.Verification.Error)
		return
	}

	v := s.Verification
	fmt.Fprintf(b, "  charges enabled: %s\n", strconv.FormatBool(v.ChargesEnabled))
	fmt.Fprintf(b, "  payouts enabled: %s\n", strconv.FormatBool(v.PayoutsEnabled))
	fmt.Fprintf(b, "  details submitted: %s\n", strconv.FormatBool(v.DetailsSubmitted))
	if v.DisabledReason != "" {
		fmt.Fprintf(b, "  disabled reason: %s\n", v.DisabledReason)
	}
	if v.CurrentlyDueCount > 0 {
		fmt.Fprintf(b, "  currently due: %d\n", v.CurrentlyDueCount)
	}
	if v.PastDueCount > 0 {
		fmt.Fprintf(b, "  past due: %d\n", v.PastDueCount)
	}
	if v.PendingVerificationCount > 0 {
		fmt.Fprintf(b, "  pending verification: %d\n", v.PendingVerificationCount)
	}
}

func writeAdminLinks(b *strings.Builder, links map[string]string) {
	fmt.Fprintln(b, "Admin links:")
	for _, key := range []string{"impersonate", "admin_user", "admin_purchases", "stripe_dashboard"} {
		if value := links[key]; value != "" {
			fmt.Fprintf(b, "  %s: %s\n", key, value)
		}
	}
}

func writeStats(b *strings.Builder, s statsInfo) {
	fmt.Fprintf(b, "Sales: %d\n", s.SalesCount)
	if s.TotalEarningsFormatted != "" {
		fmt.Fprintf(b, "Total earnings: %s\n", s.TotalEarningsFormatted)
	}
	if s.UnpaidBalanceFormatted != "" {
		fmt.Fprintf(b, "Unpaid balance: %s\n", s.UnpaidBalanceFormatted)
	}
	fmt.Fprintf(b, "Comments: %d\n", s.CommentsCount)
}
