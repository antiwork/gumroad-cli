package categories

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var product string

	cmd := &cobra.Command{
		Use:   "delete <category_id>",
		Short: "Delete a variant category",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Delete variant category "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete variant category "+args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Deleting variant category...", "DELETE", cmdutil.JoinPath("products", product, "variant_categories", args[0]), url.Values{}, "Variant category deleted.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")

	return cmd
}
