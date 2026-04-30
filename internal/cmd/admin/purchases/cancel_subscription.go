package purchases

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type cancelSubscriptionRequest struct {
	Email    string `json:"email"`
	BySeller bool   `json:"by_seller,omitempty"`
}

type cancelSubscriptionResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	CancelledAt string `json:"cancelled_at"`
}

func newCancelSubscriptionCmd() *cobra.Command {
	var (
		email    string
		bySeller bool
	)

	cmd := &cobra.Command{
		Use:   "cancel-subscription <purchase-id>",
		Short: "Cancel the subscription on a purchase as an admin",
		Long: `Cancel the subscription linked to a purchase. The buyer email is required as a
sanity check against the purchase record.

Defaults to buyer-initiated cancellation, matching the admin web UI. Pass
--by-seller to record the cancellation as seller-initiated. The cancellation is
always attributed to the admin actor regardless of the by-seller flag.`,
		Example: `  gumroad admin purchases cancel-subscription 12345 --email buyer@example.com
  gumroad admin purchases cancel-subscription 12345 --email buyer@example.com --by-seller
  gumroad admin purchases cancel-subscription 12345 --email buyer@example.com --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			path := cmdutil.JoinPath("purchases", args[0], "cancel_subscription")

			confirmMsg := "Cancel subscription for buyer " + email + " on purchase " + args[0] + "? (buyer-initiated)"
			if bySeller {
				confirmMsg = "Cancel subscription for buyer " + email + " on purchase " + args[0] + "? (seller-initiated)"
			}
			ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "cancel subscription on purchase "+args[0], args[0])
			}

			req := cancelSubscriptionRequest{Email: email, BySeller: bySeller}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), cancelSubscriptionDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Cancelling subscription...", path, req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[cancelSubscriptionResponse](data)
			if err != nil {
				return err
			}
			return renderCancelSubscription(opts, args[0], decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")
	cmd.Flags().BoolVar(&bySeller, "by-seller", false, "Record cancellation as seller-initiated (default is buyer-initiated)")

	return cmd
}

func cancelSubscriptionDryRunParams(req cancelSubscriptionRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.BySeller {
		params.Set("by_seller", "true")
	}
	return params
}

func renderCancelSubscription(opts cmdutil.Options, purchaseID string, resp cancelSubscriptionResponse) error {
	message := resp.Message
	if message == "" {
		message = "Cancelled subscription for purchase " + purchaseID
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{"true", message, purchaseID, resp.Status, resp.CancelledAt},
		})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(message)); err != nil {
		return err
	}
	if resp.Status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", resp.Status); err != nil {
			return err
		}
	}
	if resp.CancelledAt != "" {
		return output.Writef(opts.Out(), "Cancelled at: %s\n", resp.CancelledAt)
	}
	return nil
}
