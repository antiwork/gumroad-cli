package sales

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newRefundCmd() *cobra.Command {
	var amount string

	cmd := &cobra.Command{
		Use:   "refund <id>",
		Short: "Refund a sale",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			var cents int
			hasAmount := c.Flags().Changed("amount")
			if hasAmount {
				parsed, err := cmdutil.ParseMoney("amount", amount, "amount", "")
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				if parsed <= 0 {
					return cmdutil.UsageErrorf(c, "--amount must be greater than 0")
				}
				cents = parsed
			}

			isPartial := cents > 0
			amountDesc := cmdutil.FormatMoney(cents, "")

			msg := "Refund sale " + args[0] + "?"
			if isPartial {
				msg = fmt.Sprintf("Refund %s on sale %s?", amountDesc, args[0])
			}

			ok, err := cmdutil.ConfirmAction(opts, msg)
			if err != nil {
				return err
			}
			if !ok {
				action := "refund sale " + args[0]
				if isPartial {
					action = fmt.Sprintf("refund %s on sale %s", amountDesc, args[0])
				}
				return cmdutil.PrintCancelledAction(opts, action)
			}

			params := url.Values{}
			successMessage := "Sale " + args[0] + " refunded."
			if isPartial {
				params.Set("amount_cents", strconv.Itoa(cents))
				successMessage = fmt.Sprintf("Refunded %s on sale %s.", amountDesc, args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Refunding sale...", "PUT", cmdutil.JoinPath("sales", args[0], "refund"), params, successMessage)
		},
	}

	cmd.Flags().StringVar(&amount, "amount", "", "Partial refund amount (e.g. 5, 5.00)")

	return cmd
}
