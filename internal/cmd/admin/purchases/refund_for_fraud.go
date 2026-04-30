package purchases

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type refundForFraudRequest struct {
	Email string `json:"email"`
}

type refundForFraudResponse struct {
	Message                 string   `json:"message"`
	Purchase                purchase `json:"purchase"`
	SubscriptionCancelled   bool     `json:"subscription_cancelled"`
	SubscriptionCancelError string   `json:"subscription_cancel_error"`
}

func newRefundForFraudCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "refund-for-fraud <purchase-id>",
		Short: "Refund a purchase for fraud and block the buyer",
		Long: `Refund a purchase, classify the refund as fraud, and block the buyer in a
single admin action. The buyer email is required as a sanity check against the
purchase record.

This is a compound action: the full purchase amount is refunded, the buyer is
blocked, any linked subscription is cancelled, and the seller receives a fraud
notice email. There is no partial-refund flag — fraud refunds always refund the
full remaining balance.`,
		Example: `  gumroad admin purchases refund-for-fraud 12345 --email buyer@example.com
  gumroad admin purchases refund-for-fraud 12345 --email buyer@example.com --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			path := cmdutil.JoinPath("purchases", args[0], "refund_for_fraud")

			confirmMsg := fmt.Sprintf("Refund purchase %s for fraud and block buyer %s? Also cancels any linked subscription and sends a fraud notice.", args[0], email)
			ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "refund purchase "+args[0]+" for fraud and block buyer "+email, args[0])
			}

			req := refundForFraudRequest{Email: email}

			if opts.DryRun {
				params := url.Values{}
				params.Set("email", email)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Refunding purchase for fraud...", path, req)
			if err != nil {
				return wrapRefundForFraudError(args[0], err)
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[refundForFraudResponse](data)
			if err != nil {
				return err
			}
			return renderRefundForFraud(opts, args[0], decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")

	return cmd
}

func wrapRefundForFraudError(purchaseID string, err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message: fmt.Sprintf(
				"refund-for-fraud request failed: %s. Verify status with 'gumroad admin purchases view %s' before retrying to avoid duplicate refunds",
				apiErr.Message, purchaseID,
			),
			Hint: apiErr.Hint,
		}
	}
	return fmt.Errorf("refund-for-fraud request failed: %w. Verify status with 'gumroad admin purchases view %s' before retrying to avoid duplicate refunds", err, purchaseID)
}

func renderRefundForFraud(opts cmdutil.Options, purchaseID string, resp refundForFraudResponse) error {
	subscriptionStatus := refundForFraudSubscriptionStatusLabel(resp)
	headline := fallback(resp.Message, "Refunded purchase "+purchaseID+" for fraud and blocked the buyer")

	if opts.PlainOutput {
		row := []string{
			"true",
			headline,
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

func refundForFraudSubscriptionStatusLabel(resp refundForFraudResponse) string {
	switch {
	case resp.SubscriptionCancelled:
		return "cancelled"
	case resp.SubscriptionCancelError != "":
		return "cancel_failed"
	default:
		return "not_cancelled"
	}
}
