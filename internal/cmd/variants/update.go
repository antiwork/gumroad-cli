package variants

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var product, category, name, description, priceDifference string
	var maxPurchaseCount int
	var files []string
	var fileNames []string
	var fileDescriptions []string

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
			if err := cmdutil.RequireAnyFlagChanged(c,
				"name", "description", "price-difference", "max-purchase-count",
				"file", "file-name", "file-description",
			); err != nil {
				return err
			}

			flags := c.Flags()
			requestedUploads, err := collectRequestedVariantUploads(c, files, fileNames, fileDescriptions)
			if err != nil {
				return err
			}

			params := url.Values{}
			if flags.Changed("name") {
				if name == "" {
					return cmdutil.UsageErrorf(c, "--name cannot be empty")
				}
				params.Set("name", name)
			}
			if flags.Changed("description") {
				params.Set("description", description)
			}
			if flags.Changed("price-difference") {
				cents, err := cmdutil.ParseSignedMoney("price-difference", priceDifference, "price", "")
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				params.Set("price_difference_cents", strconv.Itoa(cents))
			}
			if flags.Changed("max-purchase-count") {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants", args[0])
			fileFlagsChanged := flags.Changed("file") ||
				flags.Changed("file-name") ||
				flags.Changed("file-description")
			if !fileFlagsChanged {
				return cmdutil.RunRequestWithSuccess(opts, "Updating variant...", "PUT", path, params, args[0], "Variant "+args[0]+" updated.")
			}
			if len(requestedUploads) == 0 {
				return cmdutil.UsageErrorf(c, "--file-name and --file-description require at least one --file")
			}

			return runVariantUpdateWithFiles(opts, product, args[0], path, params, requestedUploads)
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "New name")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().StringVar(&priceDifference, "price-difference", "", "New price difference (e.g. 5.00, -1.50)")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "New max purchase count")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a new local file to this variant's content (repeatable)")
	cmd.Flags().StringArrayVar(&fileNames, "file-name", nil, "Display name for the matching --file (repeatable)")
	cmd.Flags().StringArrayVar(&fileDescriptions, "file-description", nil, "Description for the matching --file (repeatable)")

	return cmd
}
