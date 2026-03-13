package payouts

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newUpcomingCmd() *cobra.Command {
	var noSales, includeTransactions bool

	cmd := &cobra.Command{
		Use:   "upcoming",
		Short: "View upcoming payout",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			params := url.Values{}
			if noSales {
				params.Set("include_sales", "false")
			}
			if includeTransactions {
				params.Set("include_transactions", "true")
			}

			return cmdutil.RunRequest(opts, "Fetching upcoming payout...", "GET", "/payouts/upcoming", params, func(data json.RawMessage) error {
				var resp struct {
					Payout struct {
						DisplayPayoutPeriod string          `json:"display_payout_period"`
						FormattedAmount     string          `json:"formatted_amount"`
						Transactions        json.RawMessage `json:"transactions"`
					} `json:"payout"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				p := resp.Payout
				style := opts.Style()

				if opts.PlainOutput {
					row := []string{p.DisplayPayoutPeriod, p.FormattedAmount}
					row = append(row, payoutPlainDetailColumns(noSales, includeTransactions, p.Transactions)...)
					return output.PrintPlain(opts.Out(), [][]string{row})
				}

				if err := output.Writef(opts.Out(), "%s %s\n", style.Bold("Upcoming payout:"), p.FormattedAmount); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Period: %s\n", p.DisplayPayoutPeriod); err != nil {
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
