package licenses

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newRotateCmd() *cobra.Command {
	var product, key string

	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate (regenerate) a license key",
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

			return cmdutil.RunRequest(opts, "Rotating license key...", "PUT", "/licenses/rotate", params, func(data json.RawMessage) error {
				var resp struct {
					Purchase struct {
						LicenseKey string `json:"license_key"`
					} `json:"purchase"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{resp.Purchase.LicenseKey},
					})
				}

				if opts.Quiet {
					return output.Writeln(opts.Out(), resp.Purchase.LicenseKey)
				}

				style := opts.Style()
				if err := output.Writeln(opts.Out(), style.Green("License key rotated.")); err != nil {
					return err
				}
				return output.Writef(opts.Out(), "New key: %s\n", resp.Purchase.LicenseKey)
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	addLicenseKeyFlag(cmd, &key)

	return cmd
}
