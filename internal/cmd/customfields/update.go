package customfields

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var product, name string
	var required bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a custom field",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}
			if err := cmdutil.RequireAnyFlagChanged(c, "required"); err != nil {
				return err
			}

			params := url.Values{}
			if c.Flags().Changed("required") {
				if required {
					params.Set("required", "true")
				} else {
					params.Set("required", "false")
				}
			}

			return cmdutil.RunRequestWithSuccess(opts, "Updating custom field...", "PUT", cmdutil.JoinPath("products", product, "custom_fields", name), params, name, "Custom field "+name+" updated.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Field name (required)")
	cmd.Flags().BoolVar(&required, "required", false, "Make field required")

	return cmd
}
