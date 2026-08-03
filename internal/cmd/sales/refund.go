package sales

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type refundSaleLookup struct {
	Sale struct {
		Currency          string `json:"currency"`
		CurrencyType      string `json:"currency_type"`
		PriceCurrencyType string `json:"price_currency_type"`
	} `json:"sale"`
}

func (l refundSaleLookup) currency() string {
	for _, value := range []string{l.Sale.Currency, l.Sale.CurrencyType, l.Sale.PriceCurrencyType} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func newRefundCmd() *cobra.Command {
	var amount string

	cmd := &cobra.Command{
		Use:   "refund <id>",
		Short: "Refund a sale",
		Long: `Refund a sale in full, or partially with --amount.

--amount is given in the sale's own currency, so the sale is looked up first to
find out what that currency is. Without the lookup a value like 25 could mean
either $25.00 or ¥25, and sending the wrong one refunds 100 times the intended
amount on a yen-priced sale.`,
		Example: `  gumroad sales refund A-m3CDDC5dlrSdKZp0RFhA==
  gumroad sales refund A-m3CDDC5dlrSdKZp0RFhA== --amount 2.00`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			var (
				cents    int
				currency string
			)
			if c.Flags().Changed("amount") {
				lookup, err := cmdutil.FetchRequestDecoded[refundSaleLookup](opts, "Looking up sale...", "GET", cmdutil.JoinPath("sales", args[0]), url.Values{})
				if err != nil {
					return err
				}
				currency = lookup.currency()
				if currency == "" {
					return cmdutil.InvalidInputErrorf("could not determine the currency of sale %s, so --amount cannot be scaled safely; re-run without --amount for a full refund", args[0])
				}

				parsed, err := cmdutil.ParseMoney("amount", amount, "amount", currency)
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				if parsed <= 0 {
					return cmdutil.UsageErrorf(c, "--amount must be greater than 0")
				}
				cents = parsed
			}

			isPartial := cents > 0
			amountDesc := ""
			if isPartial {
				amountDesc = fmt.Sprintf("%s %s", cmdutil.FormatMoney(cents, currency), strings.ToUpper(currency))
			}

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
				return cmdutil.PrintCancelledAction(opts, action, args[0])
			}

			params := url.Values{}
			successMessage := "Sale " + args[0] + " refunded."
			if isPartial {
				params.Set("amount_cents", strconv.Itoa(cents))
				successMessage = fmt.Sprintf("Refunded %s on sale %s.", amountDesc, args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Refunding sale...", "PUT", cmdutil.JoinPath("sales", args[0], "refund"), params, args[0], successMessage)
		},
	}

	cmd.Flags().StringVar(&amount, "amount", "", "Partial refund amount in the sale's currency (e.g. 5, 5.00)")

	return cmd
}
