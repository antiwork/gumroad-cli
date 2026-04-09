package customfields

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var product, name string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a custom field (keyed by name)",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Delete custom field \""+name+"\"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete custom field \""+name+"\"")
			}

			return cmdutil.RunRequestWithSuccess(opts, "Deleting custom field...", "DELETE", cmdutil.JoinPath("products", product, "custom_fields", name), nil, "Custom field "+name+" deleted.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Field name (required)")

	return cmd
}
