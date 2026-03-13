package variants

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var product, category, name, description string
	var priceDifferenceCents, maxPurchaseCount int

	cmd := &cobra.Command{
		Use:   "update <variant_id>",
		Short: "Update a variant",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if category == "" {
				return cmdutil.MissingFlagError(c, "--category")
			}
			if err := cmdutil.RequireAnyFlagChanged(c, "name", "description", "price-difference-cents", "max-purchase-count"); err != nil {
				return err
			}

			params := url.Values{}
			if c.Flags().Changed("name") {
				if name == "" {
					return cmdutil.UsageErrorf(c, "--name cannot be empty")
				}
				params.Set("name", name)
			}
			if c.Flags().Changed("description") {
				params.Set("description", description)
			}
			if c.Flags().Changed("price-difference-cents") {
				params.Set("price_difference_cents", strconv.Itoa(priceDifferenceCents))
			}
			if c.Flags().Changed("max-purchase-count") {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants", args[0])
			return cmdutil.RunRequestWithSuccess(opts, "Updating variant...", "PUT", path, params, "Variant updated.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "New name")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().IntVar(&priceDifferenceCents, "price-difference-cents", 0, "New price difference in cents")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "New max purchase count")

	return cmd
}
