package purchases

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type unblockBuyerRequest struct {
	Email string `json:"email"`
}

func newUnblockBuyerCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "unblock-buyer <purchase-id>",
		Short: "Unblock the buyer on a purchase as an admin",
		Long: `Unblock the buyer associated with a purchase. The buyer email is required as
a sanity check against the purchase record.`,
		Example: `  gumroad admin purchases unblock-buyer 12345 --email buyer@example.com
  gumroad admin purchases unblock-buyer 12345 --email buyer@example.com --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			path := cmdutil.JoinPath("purchases", args[0], "unblock_buyer")

			ok, err := cmdutil.ConfirmAction(opts, "Unblock buyer "+email+" on purchase "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "unblock buyer "+email+" on purchase "+args[0], args[0])
			}

			req := unblockBuyerRequest{Email: email}

			if opts.DryRun {
				params := url.Values{}
				params.Set("email", email)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Unblocking buyer...", path, req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[purchaseActionResponse](data)
			if err != nil {
				return err
			}
			return renderPurchaseAction(opts, args[0], decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")

	return cmd
}
