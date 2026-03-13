package categories

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var product, title string

	cmd := &cobra.Command{
		Use:   "update <category_id>",
		Short: "Update a variant category",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if err := cmdutil.RequireAnyFlagChanged(c, "title"); err != nil {
				return err
			}

			params := url.Values{}
			if c.Flags().Changed("title") {
				if title == "" {
					return cmdutil.UsageErrorf(c, "--title cannot be empty")
				}
				params.Set("title", title)
			}

			return cmdutil.RunRequestWithSuccess(opts, "Updating variant category...", "PUT", cmdutil.JoinPath("products", product, "variant_categories", args[0]), params, "Variant category updated.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&title, "title", "", "New title")

	return cmd
}
