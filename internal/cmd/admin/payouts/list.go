package payouts

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type payoutsResponse struct {
	LastPayouts          []payout `json:"last_payouts"`
	NextPayoutDate       string   `json:"next_payout_date"`
	BalanceForNextPayout string   `json:"balance_for_next_payout"`
	PayoutNote           string   `json:"payout_note"`
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
	var email string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent payouts for a user",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			params := url.Values{}
			params.Set("email", email)
			return admincmd.RunGetDecoded[payoutsResponse](opts, "Fetching payouts...", "/payouts", params, func(resp payoutsResponse) error {
				return renderPayouts(opts, email, resp)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}

func renderPayouts(opts cmdutil.Options, email string, resp payoutsResponse) error {
	if opts.PlainOutput {
		return writePayoutsPlain(opts.Out(), email, resp)
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, style.Bold(email)); err != nil {
			return err
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
		if len(resp.LastPayouts) == 0 {
			return output.Writeln(w, "No recent payouts found.")
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		return writePayoutsTable(w, style, resp.LastPayouts)
	})
}

func writePayoutsPlain(w io.Writer, email string, resp payoutsResponse) error {
	if len(resp.LastPayouts) == 0 {
		return output.PrintPlain(w, [][]string{{email, "", "", "", "", resp.NextPayoutDate, resp.BalanceForNextPayout, resp.PayoutNote}})
	}

	rows := make([][]string, 0, len(resp.LastPayouts))
	for _, p := range resp.LastPayouts {
		rows = append(rows, []string{
			email,
			p.ExternalID,
			formatAmount(p),
			p.State,
			p.CreatedAt,
			p.Processor,
			payoutDestination(p),
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
	if p.Currency == "" {
		return fmt.Sprintf("%d cents", p.AmountCents)
	}
	return strings.TrimSpace(fmt.Sprintf("%d %s cents", p.AmountCents, strings.ToUpper(p.Currency)))
}

func payoutDestination(p payout) string {
	if p.BankAccountVisual != "" {
		return p.BankAccountVisual
	}
	return p.PaypalEmail
}
