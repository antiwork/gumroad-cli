package offercodes

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var product string
	var maxPurchaseCount int

	cmd := &cobra.Command{
		Use:   "update <code_id>",
		Short: "Update an offer code",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if err := cmdutil.RequireAnyFlagChanged(c, "max-purchase-count"); err != nil {
				return err
			}

			params := url.Values{}
			if c.Flags().Changed("max-purchase-count") {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}

			return cmdutil.RunRequestWithSuccess(opts, "Updating offer code...", "PUT", cmdutil.JoinPath("products", product, "offer_codes", args[0]), params, "Offer code updated.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "Maximum number of uses")

	return cmd
}
