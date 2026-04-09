package licenses

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDecrementCmd() *cobra.Command {
	var product, key string

	cmd := &cobra.Command{
		Use:   "decrement",
		Short: "Decrement the use count of a license key",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			key, err := resolveLicenseKey(c, opts, key)
			if err != nil {
				return err
			}

			params := url.Values{}
			params.Set("product_id", product)
			params.Set("license_key", key)

			return cmdutil.RunRequestWithSuccess(opts, "Decrementing license uses...", "PUT", "/licenses/decrement_uses_count", params, "License use count decremented for product "+product+".")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	addLicenseKeyFlag(cmd, &key)

	return cmd
}
