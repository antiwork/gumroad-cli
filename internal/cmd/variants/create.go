package variants

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var product, category, name, description string
	var priceDifferenceCents, maxPurchaseCount int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a variant",
		Args:  cmdutil.ExactArgs(0),
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
			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}

			flags := c.Flags()
			hasPriceDifference := flags.Changed("price-difference-cents")
			hasMaxPurchaseCount := flags.Changed("max-purchase-count")

			params := url.Values{}
			params.Set("name", name)
			if description != "" {
				params.Set("description", description)
			}
			if hasPriceDifference {
				params.Set("price_difference_cents", strconv.Itoa(priceDifferenceCents))
			}
			if hasMaxPurchaseCount {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants")
			return cmdutil.RunRequestWithSuccess(opts, "Creating variant...", "POST", path, params, "Variant created.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Variant name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Variant description")
	cmd.Flags().IntVar(&priceDifferenceCents, "price-difference-cents", 0, "Price difference in cents")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "Maximum number of purchases")

	return cmd
}
