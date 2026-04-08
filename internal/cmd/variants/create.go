package variants

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var product, category, name, description, priceDifference string
	var maxPurchaseCount int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a variant",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
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
			hasPriceDifference := flags.Changed("price-difference")
			hasMaxPurchaseCount := flags.Changed("max-purchase-count")

			params := url.Values{}
			params.Set("name", name)
			if description != "" {
				params.Set("description", description)
			}
			if hasPriceDifference {
				cents, err := cmdutil.ParseSignedMoney("price-difference", priceDifference, "price", "")
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				params.Set("price_difference_cents", strconv.Itoa(cents))
			}
			if hasMaxPurchaseCount {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants")
			return cmdutil.RunRequestWithSuccess(cmdutil.OptionsFrom(c), "Creating variant...", "POST", path, params, "Variant created.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Variant name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Variant description")
	cmd.Flags().StringVar(&priceDifference, "price-difference", "", "Price difference (e.g. 5.00, -1.50)")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "Maximum number of purchases")

	return cmd
}
