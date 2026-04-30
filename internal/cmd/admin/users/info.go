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

type infoRequest struct {
	Email string `json:"email"`
}

type infoResponse struct {
	User userInfo `json:"user"`
}

type userInfo struct {
	ID                             string      `json:"id"`
	Email                          string      `json:"email"`
	Name                           string      `json:"name"`
	Username                       string      `json:"username"`
	ProfileURL                     string      `json:"profile_url"`
	Country                        string      `json:"country"`
	CreatedAt                      string      `json:"created_at"`
	DeletedAt                      string      `json:"deleted_at"`
	RiskState                      riskState   `json:"risk_state"`
	TwoFactorAuthenticationEnabled bool        `json:"two_factor_authentication_enabled"`
	Payouts                        payoutsInfo `json:"payouts"`
	Stats                          statsInfo   `json:"stats"`
}

type riskState struct {
	Status                 string `json:"status"`
	UserRiskState          string `json:"user_risk_state"`
	Suspended              bool   `json:"suspended"`
	FlaggedForFraud        bool   `json:"flagged_for_fraud"`
	FlaggedForTOSViolation bool   `json:"flagged_for_tos_violation"`
	OnProbation            bool   `json:"on_probation"`
	Compliant              bool   `json:"compliant"`
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

func newInfoCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "info",
		Short: "View a comprehensive admin info payload for a user",
		Long: `View a comprehensive admin info payload for a user, combining identity,
risk state, two-factor state, payouts state, and earnings/sales stats into a
single response. Mirrors the admin web user-detail page so support workflows
can resolve from a single CLI invocation.`,
		Example: `  gumroad admin users info --email user@example.com
  gumroad admin users info --email user@example.com --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			return admincmd.RunPostJSONDecoded[infoResponse](opts, "Fetching user info...", "/users/info", infoRequest{Email: email}, func(resp infoResponse) error {
				return renderInfo(opts, email, resp.User)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}

func renderInfo(opts cmdutil.Options, email string, info userInfo) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			fallback(info.Email, email),
			info.Name,
			info.Username,
			info.RiskState.Status,
			strconv.FormatBool(info.TwoFactorAuthenticationEnabled),
			strconv.FormatBool(info.Payouts.PausedInternally),
			strconv.FormatBool(info.Payouts.PausedByUser),
			info.Payouts.NextPayoutDate,
			strconv.Itoa(info.Stats.SalesCount),
			info.Stats.TotalEarningsFormatted,
			info.CreatedAt,
		}})
	}

	if opts.Quiet {
		return nil
	}

	var b strings.Builder
	style := opts.Style()

	headline := info.Name
	if headline == "" {
		headline = fallback(info.Email, email)
	}
	fmt.Fprintln(&b, style.Bold(headline))
	writeOptional(&b, "Email", info.Email)
	writeOptional(&b, "Username", info.Username)
	writeOptional(&b, "Profile", info.ProfileURL)
	writeOptional(&b, "Country", info.Country)
	writeOptional(&b, "Created", info.CreatedAt)
	writeOptional(&b, "Deleted", info.DeletedAt)

	fmt.Fprintln(&b)
	writeRiskState(&b, info.RiskState)

	twoFactor := "disabled"
	if info.TwoFactorAuthenticationEnabled {
		twoFactor = "enabled"
	}
	fmt.Fprintf(&b, "Two-factor: %s\n", twoFactor)

	fmt.Fprintln(&b)
	writePayouts(&b, info.Payouts)

	fmt.Fprintln(&b)
	writeStats(&b, info.Stats)

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
	if risk.UserRiskState != "" && risk.UserRiskState != risk.Status {
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
