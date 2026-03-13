package payouts

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	var noSales, includeTransactions bool

	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "View a payout",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			params := url.Values{}
			if noSales {
				params.Set("include_sales", "false")
			}
			if includeTransactions {
				params.Set("include_transactions", "true")
			}

			return cmdutil.RunRequest(opts, "Fetching payout...", "GET", cmdutil.JoinPath("payouts", args[0]), params, func(data json.RawMessage) error {
				var resp struct {
					Payout struct {
						ID                  string          `json:"id"`
						DisplayPayoutPeriod string          `json:"display_payout_period"`
						FormattedAmount     string          `json:"formatted_amount"`
						IsUpcoming          bool            `json:"is_upcoming"`
						Transactions        json.RawMessage `json:"transactions"`
					} `json:"payout"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				p := resp.Payout
				style := opts.Style()

				if opts.PlainOutput {
					row := []string{p.ID, p.DisplayPayoutPeriod, p.FormattedAmount}
					row = append(row, payoutPlainDetailColumns(noSales, includeTransactions, p.Transactions)...)
					return output.PrintPlain(opts.Out(), [][]string{row})
				}

				status := ""
				if p.IsUpcoming {
					status = " " + style.Yellow("(upcoming)")
				}
				if err := output.Writef(opts.Out(), "%s%s\n", style.Bold(p.DisplayPayoutPeriod), status); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "ID: %s\n", p.ID); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Amount: %s\n", p.FormattedAmount); err != nil {
					return err
				}
				return writePayoutDetailLines(opts.Out(), noSales, includeTransactions, p.Transactions)
			})
		},
	}

	cmd.Flags().BoolVar(&noSales, "no-sales", false, "Exclude sales from payout details")
	cmd.Flags().BoolVar(&includeTransactions, "include-transactions", false, "Include transactions")

	return cmd
}
