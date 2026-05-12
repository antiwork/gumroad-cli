package payouts

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type scheduledPayout struct {
	ExternalID             string                    `json:"external_id"`
	User                   scheduledPayoutUser       `json:"user"`
	AmountCents            api.JSONInt               `json:"payout_amount_cents"`
	Status                 string                    `json:"status"`
	Action                 string                    `json:"action"`
	ScheduledAt            string                    `json:"scheduled_at"`
	ExecutedAt             string                    `json:"executed_at"`
	CreatedAt              string                    `json:"created_at"`
	CreatedBy              scheduledPayoutCreator    `json:"created_by"`
	ProductCount           api.JSONInt               `json:"product_count"`
	IncomingAffiliateCount api.JSONInt               `json:"incoming_affiliate_count"`
	RiskState              scheduledPayoutRiskState  `json:"risk_state"`
	TopCategories          []scheduledPayoutCategory `json:"top_categories"`
	UnpaidBalanceCents     api.JSONInt               `json:"unpaid_balance_cents"`
	UnpaidBalanceFormatted string                    `json:"unpaid_balance_formatted"`
}

type scheduledPayoutUser struct {
	Email      string `json:"email"`
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
}

type scheduledPayoutCreator struct {
	Name string `json:"name"`
}

type scheduledPayoutRiskState struct {
	Status                 string `json:"status"`
	UserRiskState          string `json:"user_risk_state"`
	Suspended              bool   `json:"suspended"`
	FlaggedForFraud        bool   `json:"flagged_for_fraud"`
	FlaggedForTOSViolation bool   `json:"flagged_for_tos_violation"`
	OnProbation            bool   `json:"on_probation"`
	Compliant              bool   `json:"compliant"`
	LastStatusChangedAt    string `json:"last_status_changed_at"`
}

type scheduledPayoutCategory struct {
	Slug         string      `json:"slug"`
	ProductCount api.JSONInt `json:"product_count"`
}

type scheduledListResponse struct {
	ScheduledPayouts []scheduledPayout `json:"scheduled_payouts"`
	Pagination       cursor.Pagination `json:"pagination"`
	Limit            api.JSONInt       `json:"limit"`
}

func newScheduledListCmd() *cobra.Command {
	var (
		statuses []string
		page     cursor.Flags
		lookup   lookupFlags
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ScheduledPayout rows",
		Long: `List ScheduledPayout rows. Filter by --status (pending, executed,
cancelled, flagged, held); repeat --status to match any of several values.
Filter to a single user with --email, --user-id, or --external-id. Default
limit is 20, capped server-side at 50.`,
		Example: `  gumroad admin payouts scheduled list
  gumroad admin payouts scheduled list --status flagged
  gumroad admin payouts scheduled list --status held --status flagged
  gumroad admin payouts scheduled list --user-id 2245593582708
  gumroad admin payouts scheduled list --status pending --limit 50 --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			params := url.Values{}
			normalizedStatuses, err := normalizeStatuses(c, statuses)
			if err != nil {
				return err
			}
			for _, s := range normalizedStatuses {
				params.Add("status[]", s)
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", page.Limit); err != nil {
				return err
			}
			cursor.Apply(params, page)
			if c.Flags().Changed("email") || c.Flags().Changed("user-id") || c.Flags().Changed("external-id") {
				target, err := resolveLookupTarget(c, lookup)
				if err != nil {
					return err
				}
				if target.Email != "" {
					params.Set("email", target.Email)
				}
				if target.UserID != "" {
					params.Set("user_id", target.UserID)
				}
			}

			return admincmd.RunGetDecoded[scheduledListResponse](opts, "Fetching scheduled payouts...", "/scheduled_payouts", params, func(resp scheduledListResponse) error {
				return renderScheduledList(opts, normalizedStatuses, resp)
			})
		},
	}

	cmd.Flags().StringSliceVar(&statuses, "status", nil, "Filter by status: pending, executed, cancelled, flagged, held (repeatable)")
	cursor.AddFlags(cmd, &page)
	addLookupFlags(cmd, &lookup)

	return cmd
}

func normalizeStatuses(cmd *cobra.Command, statuses []string) ([]string, error) {
	out := make([]string, 0, len(statuses))
	for _, s := range statuses {
		normalized := strings.ToLower(strings.TrimSpace(s))
		if normalized == "" {
			continue
		}
		if _, ok := validScheduledStatuses[normalized]; !ok {
			return nil, cmdutil.UsageErrorf(cmd, "--status must be one of: %s", validStatusesList())
		}
		out = append(out, normalized)
	}
	return out, nil
}

func validStatusesList() string {
	keys := make([]string, 0, len(validScheduledStatuses))
	for k := range validScheduledStatuses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func renderScheduledList(opts cmdutil.Options, statuses []string, resp scheduledListResponse) error {
	if opts.PlainOutput {
		rows := make([][]string, 0, len(resp.ScheduledPayouts))
		for _, p := range resp.ScheduledPayouts {
			rows = append(rows, []string{
				p.ExternalID, p.User.Email, formatScheduledAmount(p), p.Status, p.Action, p.ScheduledAt, p.CreatedAt,
				p.RiskState.Status,
				strconv.Itoa(int(p.ProductCount)),
				strconv.Itoa(int(p.IncomingAffiliateCount)),
				p.UnpaidBalanceFormatted,
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	if opts.Quiet {
		return nil
	}

	statusLabel := strings.Join(statuses, ", ")

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.ScheduledPayouts) == 0 {
			if statusLabel != "" {
				return output.Writef(w, "No scheduled payouts found for status %q.\n", statusLabel)
			}
			return output.Writeln(w, "No scheduled payouts found.")
		}

		headline := fmt.Sprintf("%d scheduled payout(s)", len(resp.ScheduledPayouts))
		if statusLabel != "" {
			headline = fmt.Sprintf("%d scheduled payout(s) with status %s", len(resp.ScheduledPayouts), statusLabel)
		}
		if err := output.Writeln(w, style.Bold(headline)); err != nil {
			return err
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}

		tbl := output.NewStyledTable(style, "ID", "EMAIL", "AMOUNT", "STATUS", "SCHEDULED", "RISK", "PRODS", "AFFS", "UNPAID")
		for _, p := range resp.ScheduledPayouts {
			tbl.AddRow(
				p.ExternalID,
				p.User.Email,
				formatScheduledAmount(p),
				p.Status,
				p.ScheduledAt,
				p.RiskState.Status,
				strconv.Itoa(int(p.ProductCount)),
				strconv.Itoa(int(p.IncomingAffiliateCount)),
				p.UnpaidBalanceFormatted,
			)
		}
		if err := tbl.Render(w); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func formatScheduledAmount(p scheduledPayout) string {
	return fmt.Sprintf("%d cents", p.AmountCents)
}
