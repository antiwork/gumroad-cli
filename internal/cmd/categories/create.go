package categories

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var product, title string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a variant category",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if title == "" {
				return cmdutil.MissingFlagError(c, "--title")
			}

			params := url.Values{}
			params.Set("title", title)

			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestWithSuccess(opts, "Creating variant category...", "POST", cmdutil.JoinPath("products", product, "variant_categories"), params, "Variant category created.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&title, "title", "", "Category title (required)")

	return cmd
}
