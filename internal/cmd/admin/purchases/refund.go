package purchases

import (
	"fmt"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type refundRequest struct {
	Email              string `json:"email"`
	AmountCents        int    `json:"amount_cents,omitempty"`
	Force              bool   `json:"force,omitempty"`
	CancelSubscription bool   `json:"cancel_subscription,omitempty"`
}

type refundResponse struct {
	Message                 string   `json:"message"`
	Purchase                purchase `json:"purchase"`
	SubscriptionCancelled   bool     `json:"subscription_cancelled"`
	SubscriptionCancelError string   `json:"subscription_cancel_error"`
}

func newRefundCmd() *cobra.Command {
	var (
		email              string
		amount             string
		force              bool
		cancelSubscription bool
	)

	cmd := &cobra.Command{
		Use:   "refund <purchase-id>",
		Short: "Refund a purchase as an admin",
		Long: `Refund a specific purchase end-to-end without going through the admin web UI.

The buyer email is required as a sanity check against the purchase. Without --amount,
the entire purchase is refunded. --force bypasses the refund-policy timeframe and
fine-print guards (the active-chargeback guard still applies). --cancel-subscription
cancels the linked subscription after a successful refund.`,
		Example: `  gumroad admin purchases refund 12345 --email buyer@example.com
  gumroad admin purchases refund 12345 --email buyer@example.com --amount 5.00
  gumroad admin purchases refund 12345 --email buyer@example.com --force
  gumroad admin purchases refund 12345 --email buyer@example.com --cancel-subscription`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			var cents int
			if c.Flags().Changed("amount") {
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

			msg := "Refund purchase " + args[0] + "?"
			if isPartial {
				msg = fmt.Sprintf("Refund %s on purchase %s?", amountDesc, args[0])
			}
			if cancelSubscription {
				msg += " (will also cancel the linked subscription)"
			}
			if force {
				msg += " (forcing past refund-policy guards)"
			}

			ok, err := cmdutil.ConfirmAction(opts, msg)
			if err != nil {
				return err
			}
			if !ok {
				action := "refund purchase " + args[0]
				if isPartial {
					action = fmt.Sprintf("refund %s on purchase %s", amountDesc, args[0])
				}
				return cmdutil.PrintCancelledAction(opts, action, args[0])
			}

			req := refundRequest{
				Email:              email,
				AmountCents:        cents,
				Force:              force,
				CancelSubscription: cancelSubscription,
			}

			path := cmdutil.JoinPath("purchases", args[0], "refund")
			return admincmd.RunPostJSONDecoded[refundResponse](opts, "Refunding purchase...", path, req, func(resp refundResponse) error {
				return renderRefund(opts, args[0], resp)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")
	cmd.Flags().StringVar(&amount, "amount", "", "Partial refund amount in displayed currency (e.g. 5, 5.00); omit for full refund")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass refund-policy timeframe and fine-print guards")
	cmd.Flags().BoolVar(&cancelSubscription, "cancel-subscription", false, "Cancel the linked subscription after the refund succeeds")

	return cmd
}

func renderRefund(opts cmdutil.Options, purchaseID string, resp refundResponse) error {
	subscriptionStatus := subscriptionStatusLabel(resp)

	if opts.PlainOutput {
		row := []string{
			"true",
			fallback(resp.Message, "Refunded purchase "+purchaseID),
			fallback(resp.Purchase.ID, purchaseID),
			subscriptionStatus,
			resp.SubscriptionCancelError,
		}
		return output.PrintPlain(opts.Out(), [][]string{row})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	headline := fallback(resp.Message, "Refunded purchase "+purchaseID)
	if err := output.Writeln(opts.Out(), style.Green(headline)); err != nil {
		return err
	}

	if resp.Purchase.ID != "" {
		if err := renderPurchase(opts, resp.Purchase); err != nil {
			return err
		}
	}

	if resp.SubscriptionCancelled {
		return output.Writeln(opts.Out(), "Subscription: cancelled")
	}
	if resp.SubscriptionCancelError != "" {
		return output.Writef(opts.Out(), "Subscription cancel failed: %s\n", resp.SubscriptionCancelError)
	}
	return nil
}

func subscriptionStatusLabel(resp refundResponse) string {
	switch {
	case resp.SubscriptionCancelled:
		return "cancelled"
	case resp.SubscriptionCancelError != "":
		return "cancel_failed"
	default:
		return "not_cancelled"
	}
}

func fallback(value, alt string) string {
	if value == "" {
		return alt
	}
	return value
}
