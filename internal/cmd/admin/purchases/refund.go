package purchases

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
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

			refundPath := cmdutil.JoinPath("purchases", args[0], "refund")

			var (
				cents           int
				currency        string
				refundableCents int
				haveRefundable  bool
			)
			if c.Flags().Changed("amount") {
				lookup, err := admincmd.FetchGetDecoded[purchaseResponse](opts, "Looking up purchase...", cmdutil.JoinPath("purchases", args[0]), url.Values{})
				if err != nil {
					return err
				}
				currency = lookup.Purchase.CurrencyType
				if currency == "" {
					return fmt.Errorf("could not determine purchase currency from admin lookup; refusing --amount to avoid mis-scaled refund (re-run without --amount for a full refund, or upgrade the server to expose currency_type)")
				}
				if lookup.Purchase.AmountRefundableCentsInCurrency > 0 {
					refundableCents = int(lookup.Purchase.AmountRefundableCentsInCurrency)
					haveRefundable = true
				}

				parsed, err := cmdutil.ParseMoney("amount", amount, "amount", currency)
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				if parsed <= 0 {
					return cmdutil.UsageErrorf(c, "--amount must be greater than 0")
				}
				if haveRefundable && parsed > refundableCents {
					return cmdutil.UsageErrorf(c, "--amount %s exceeds the refundable balance of %s",
						cmdutil.FormatMoney(parsed, currency), cmdutil.FormatMoney(refundableCents, currency))
				}
				cents = parsed
			}

			isPartial := cents > 0
			amountDesc := cmdutil.FormatMoney(cents, currency)

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

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(refundPath), refundDryRunParams(req))
			}

			err = admincmd.RunPostJSONDecoded[refundResponse](opts, "Refunding purchase...", refundPath, req, func(resp refundResponse) error {
				return renderRefund(opts, args[0], resp)
			})
			if err != nil {
				return wrapRefundError(args[0], err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")
	cmd.Flags().StringVar(&amount, "amount", "", "Partial refund amount in displayed currency (e.g. 5, 5.00); omit for full refund")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass refund-policy timeframe and fine-print guards")
	cmd.Flags().BoolVar(&cancelSubscription, "cancel-subscription", false, "Cancel the linked subscription after the refund succeeds")

	return cmd
}

// wrapRefundError adds an explicit verification hint to refund POST failures.
// Unlike list/lookup commands, a failed refund could still have partially
// landed server-side, so the operator should confirm state before retrying.
func wrapRefundError(purchaseID string, err error) error {
	return fmt.Errorf("refund request failed: %w. Verify status with 'gumroad admin purchases view %s' before retrying to avoid duplicate refunds", err, purchaseID)
}

func refundDryRunParams(req refundRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.AmountCents > 0 {
		params.Set("amount_cents", strconv.Itoa(req.AmountCents))
	}
	if req.Force {
		params.Set("force", "true")
	}
	if req.CancelSubscription {
		params.Set("cancel_subscription", "true")
	}
	return params
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
