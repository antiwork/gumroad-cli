package offercodes

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var product string

	cmd := &cobra.Command{
		Use:   "delete <code_id>",
		Short: "Delete an offer code",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Delete offer code "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete offer code "+args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Deleting offer code...", "DELETE", cmdutil.JoinPath("products", product, "offer_codes", args[0]), url.Values{}, "Offer code "+args[0]+" deleted.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")

	return cmd
}
