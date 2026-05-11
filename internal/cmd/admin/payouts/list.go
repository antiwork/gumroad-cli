package payouts

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type payoutsResponse struct {
	UserID               string            `json:"user_id"`
	RecentPayouts        []payout          `json:"recent_payouts"`
	Pagination           payoutsPagination `json:"pagination"`
	NextPayoutDate       string            `json:"next_payout_date"`
	BalanceForNextPayout string            `json:"balance_for_next_payout"`
	PayoutNote           string            `json:"payout_note"`
}

type payoutsPagination struct {
	Next  string      `json:"next"`
	Limit api.JSONInt `json:"limit"`
}

type payout struct {
	ExternalID        string      `json:"external_id"`
	AmountCents       api.JSONInt `json:"amount_cents"`
	Currency          string      `json:"currency"`
	State             string      `json:"state"`
	CreatedAt         string      `json:"created_at"`
	Processor         string      `json:"processor"`
	BankAccountVisual string      `json:"bank_account_visual"`
	PaypalEmail       string      `json:"paypal_email"`
}

func newListCmd() *cobra.Command {
	var (
		lookup lookupFlags
		limit  int
		cursor string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent payouts for a user",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveLookupTarget(c, lookup)
			if err != nil {
				return err
			}

			params := url.Values{}
			if target.Email != "" {
				params.Set("email", target.Email)
			}
			if target.UserID != "" {
				params.Set("user_id", target.UserID)
			}
			if c.Flags().Changed("limit") {
				if err := cmdutil.RequirePositiveIntFlag(c, "limit", limit); err != nil {
					return err
				}
				params.Set("limit", strconv.Itoa(limit))
			}
			if cursor != "" {
				params.Set("cursor", cursor)
			}

			return admincmd.RunGetDecoded[payoutsResponse](opts, "Fetching payouts...", "/payouts", params, func(resp payoutsResponse) error {
				return renderPayouts(opts, target.identifier(), resp)
			})
		},
	}

	addLookupFlags(cmd, &lookup)
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum results per page (default 20)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (from a previous response)")

	return cmd
}

func renderPayouts(opts cmdutil.Options, identifier string, resp payoutsResponse) error {
	if opts.PlainOutput {
		return writePayoutsPlain(opts.Out(), identifier, resp)
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, style.Bold(identifier)); err != nil {
			return err
		}
		if resp.UserID != "" && resp.UserID != identifier {
			if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
				return err
			}
		}
		if resp.NextPayoutDate != "" {
			if err := output.Writef(w, "Next payout: %s\n", resp.NextPayoutDate); err != nil {
				return err
			}
		}
		if resp.BalanceForNextPayout != "" {
			if err := output.Writef(w, "Balance for next payout: %s\n", resp.BalanceForNextPayout); err != nil {
				return err
			}
		}
		if resp.PayoutNote != "" {
			if err := output.Writef(w, "Payout note: %s\n", resp.PayoutNote); err != nil {
				return err
			}
		}
		if len(resp.RecentPayouts) == 0 {
			return output.Writeln(w, "No recent payouts found.")
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		if err := writePayoutsTable(w, style, resp.RecentPayouts); err != nil {
			return err
		}
		if resp.Pagination.Next != "" {
			return output.Writef(w, "\nMore results: --cursor %s\n", resp.Pagination.Next)
		}
		return nil
	})
}

func writePayoutsPlain(w io.Writer, identifier string, resp payoutsResponse) error {
	if len(resp.RecentPayouts) == 0 {
		return output.PrintPlain(w, [][]string{{identifier, "", "", "", "", "", "", resp.NextPayoutDate, resp.BalanceForNextPayout, resp.PayoutNote}})
	}

	rows := make([][]string, 0, len(resp.RecentPayouts))
	for _, p := range resp.RecentPayouts {
		rows = append(rows, []string{
			identifier,
			p.ExternalID,
			formatAmount(p),
			p.State,
			p.CreatedAt,
			p.Processor,
			payoutDestination(p),
			resp.NextPayoutDate,
			resp.BalanceForNextPayout,
			resp.PayoutNote,
		})
	}
	return output.PrintPlain(w, rows)
}

func writePayoutsTable(w io.Writer, style output.Styler, payouts []payout) error {
	tbl := output.NewStyledTable(style, "ID", "AMOUNT", "STATE", "DATE", "PROCESSOR", "DESTINATION")
	for _, p := range payouts {
		tbl.AddRow(p.ExternalID, formatAmount(p), p.State, p.CreatedAt, p.Processor, payoutDestination(p))
	}
	return tbl.Render(w)
}

func formatAmount(p payout) string {
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		return fmt.Sprintf("%d cents", p.AmountCents)
	}
	return fmt.Sprintf("%d %s cents", p.AmountCents, strings.ToUpper(currency))
}

func payoutDestination(p payout) string {
	if p.BankAccountVisual != "" {
		return p.BankAccountVisual
	}
	return p.PaypalEmail
}
