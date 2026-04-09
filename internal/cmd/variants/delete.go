package variants

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var product, category string

	cmd := &cobra.Command{
		Use:   "delete <variant_id>",
		Short: "Delete a variant",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if category == "" {
				return cmdutil.MissingFlagError(c, "--category")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Delete variant "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete variant "+args[0])
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants", args[0])
			return cmdutil.RunRequestWithSuccess(opts, "Deleting variant...", "DELETE", path, url.Values{}, "Variant "+args[0]+" deleted.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")

	return cmd
}
