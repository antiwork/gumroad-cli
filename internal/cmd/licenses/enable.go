package licenses

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newEnableCmd() *cobra.Command {
	var product, key string

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable a license key",
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

			return cmdutil.RunRequestWithSuccess(opts, "Enabling license...", "PUT", "/licenses/enable", params, "License enabled for product "+product+".")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	addLicenseKeyFlag(cmd, &key)

	return cmd
}
