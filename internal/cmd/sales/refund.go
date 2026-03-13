package sales

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newRefundCmd() *cobra.Command {
	var amountCents int

	cmd := &cobra.Command{
		Use:   "refund <id>",
		Short: "Refund a sale",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequirePositiveIntFlag(c, "amount-cents", amountCents); err != nil {
				return err
			}

			msg := "Refund sale " + args[0] + "?"
			if amountCents > 0 {
				msg = fmt.Sprintf("Refund %d cents on sale %s?", amountCents, args[0])
			}

			ok, err := cmdutil.ConfirmAction(opts, msg)
			if err != nil {
				return err
			}
			if !ok {
				action := "refund sale " + args[0]
				if amountCents > 0 {
					action = fmt.Sprintf("refund %d cents on sale %s", amountCents, args[0])
				}
				return cmdutil.PrintCancelledAction(opts, action)
			}

			params := url.Values{}
			successMessage := "Sale " + args[0] + " refunded."
			if amountCents > 0 {
				params.Set("amount_cents", strconv.Itoa(amountCents))
				successMessage = fmt.Sprintf("Refunded %d cents on sale %s.", amountCents, args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Refunding sale...", "PUT", cmdutil.JoinPath("sales", args[0], "refund"), params, successMessage)
		},
	}

	cmd.Flags().IntVar(&amountCents, "amount-cents", 0, "Partial refund amount in cents")

	return cmd
}
