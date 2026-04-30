package purchases

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type blockBuyerRequest struct {
	Email          string `json:"email"`
	CommentContent string `json:"comment_content,omitempty"`
}

func newBlockBuyerCmd() *cobra.Command {
	var (
		email   string
		comment string
	)

	cmd := &cobra.Command{
		Use:   "block-buyer <purchase-id>",
		Short: "Block the buyer on a purchase as an admin",
		Long: `Block the buyer associated with a purchase. The buyer email is required as a
sanity check against the purchase record. The block records an audit comment on
the purchase; pass --comment to set its content (otherwise the server records a
default comment).`,
		Example: `  gumroad admin purchases block-buyer 12345 --email buyer@example.com
  gumroad admin purchases block-buyer 12345 --email buyer@example.com --comment "Refund abuse"
  gumroad admin purchases block-buyer 12345 --email buyer@example.com --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			path := cmdutil.JoinPath("purchases", args[0], "block_buyer")

			ok, err := cmdutil.ConfirmAction(opts, "Block buyer "+email+" on purchase "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "block buyer "+email+" on purchase "+args[0], args[0])
			}

			req := blockBuyerRequest{Email: email, CommentContent: comment}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), blockBuyerDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Blocking buyer...", path, req)
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
	cmd.Flags().StringVar(&comment, "comment", "", "Audit comment recorded against the purchase")

	return cmd
}

func blockBuyerDryRunParams(req blockBuyerRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.CommentContent != "" {
		params.Set("comment_content", req.CommentContent)
	}
	return params
}
