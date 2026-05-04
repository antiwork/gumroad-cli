package payouts

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type scheduledListRequest struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type scheduledPayout struct {
	ExternalID  string      `json:"external_id"`
	Email       string      `json:"email"`
	AmountCents api.JSONInt `json:"amount_cents"`
	Currency    string      `json:"currency"`
	Status      string      `json:"status"`
	Processor   string      `json:"processor"`
	ScheduledAt string      `json:"scheduled_at"`
	CreatedAt   string      `json:"created_at"`
}

type scheduledListResponse struct {
	ScheduledPayouts []scheduledPayout `json:"scheduled_payouts"`
	Limit            api.JSONInt       `json:"limit"`
}

func newScheduledListCmd() *cobra.Command {
	var (
		status string
		limit  int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ScheduledPayout rows",
		Long: `List ScheduledPayout rows. Filter by --status (pending, executed,
cancelled, flagged, held). Default limit is 20, capped server-side at 50.`,
		Example: `  gumroad admin payouts scheduled list
  gumroad admin payouts scheduled list --status flagged
  gumroad admin payouts scheduled list --status pending --limit 50 --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			req := scheduledListRequest{}
			if c.Flags().Changed("status") {
				normalized := strings.ToLower(strings.TrimSpace(status))
				if _, ok := validScheduledStatuses[normalized]; !ok {
					return cmdutil.UsageErrorf(c, "--status must be one of: %s", validStatusesList())
				}
				req.Status = normalized
			}
			if c.Flags().Changed("limit") {
				if err := cmdutil.RequirePositiveIntFlag(c, "limit", limit); err != nil {
					return err
				}
				req.Limit = limit
			}

			return admincmd.RunPostJSONDecoded[scheduledListResponse](opts, "Fetching scheduled payouts...", "/payouts/scheduled_list", req, func(resp scheduledListResponse) error {
				return renderScheduledList(opts, req.Status, resp)
			})
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status: pending, executed, cancelled, flagged, held")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum results to return (default 20, capped at 50)")

	return cmd
}

func validStatusesList() string {
	keys := make([]string, 0, len(validScheduledStatuses))
	for k := range validScheduledStatuses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func renderScheduledList(opts cmdutil.Options, status string, resp scheduledListResponse) error {
	if opts.PlainOutput {
		rows := make([][]string, 0, len(resp.ScheduledPayouts))
		for _, p := range resp.ScheduledPayouts {
			rows = append(rows, []string{
				p.ExternalID, p.Email, formatScheduledAmount(p), p.Status, p.Processor, p.ScheduledAt, p.CreatedAt,
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.ScheduledPayouts) == 0 {
			if status != "" {
				return output.Writef(w, "No scheduled payouts found for status %q.\n", status)
			}
			return output.Writeln(w, "No scheduled payouts found.")
		}

		headline := fmt.Sprintf("%d scheduled payout(s)", len(resp.ScheduledPayouts))
		if status != "" {
			headline = fmt.Sprintf("%d scheduled payout(s) with status %s", len(resp.ScheduledPayouts), status)
		}
		if err := output.Writeln(w, style.Bold(headline)); err != nil {
			return err
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}

		tbl := output.NewStyledTable(style, "ID", "EMAIL", "AMOUNT", "STATUS", "PROCESSOR", "SCHEDULED")
		for _, p := range resp.ScheduledPayouts {
			tbl.AddRow(p.ExternalID, p.Email, formatScheduledAmount(p), p.Status, p.Processor, p.ScheduledAt)
		}
		return tbl.Render(w)
	})
}

func formatScheduledAmount(p scheduledPayout) string {
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		return fmt.Sprintf("%d cents", p.AmountCents)
	}
	return fmt.Sprintf("%d %s cents", p.AmountCents, strings.ToUpper(currency))
}
