package licenses

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var product, key string
	var noIncrement bool

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a license key",
		Args:  cmdutil.ExactArgs(0),
		Long: `Verify a license key for a product.

WARNING: By default, this increments the license use count.
Use --no-increment to verify without incrementing.

Provide the key via stdin or the interactive prompt when possible. --key remains for compatibility.`,
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
			if noIncrement {
				params.Set("increment_uses_count", "false")
			}

			return cmdutil.RunRequest(opts, "Verifying license...", "POST", "/licenses/verify", params, func(data json.RawMessage) error {
				var resp struct {
					Uses     api.JSONInt `json:"uses"`
					Purchase struct {
						Email     string `json:"email"`
						ProductID string `json:"product_id"`
					} `json:"purchase"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				p := resp.Purchase

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{p.Email, p.ProductID, fmt.Sprintf("%d", resp.Uses)},
					})
				}

				if opts.Quiet {
					return nil
				}

				style := opts.Style()
				if err := output.Writeln(opts.Out(), style.Green("License valid")); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Email: %s\n", p.Email); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Product: %s\n", p.ProductID); err != nil {
					return err
				}
				return output.Writef(opts.Out(), "Uses: %d\n", resp.Uses)
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	addLicenseKeyFlag(cmd, &key)
	cmd.Flags().BoolVar(&noIncrement, "no-increment", false, "Verify without incrementing use count")

	return cmd
}
