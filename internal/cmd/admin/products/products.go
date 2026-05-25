package products

import "github.com/spf13/cobra"

func NewProductsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "products",
		Short: "Read admin product records",
		Example: `  gumroad admin products list --email seller@example.com
  gumroad admin products list --external-id 2245593582708
  gumroad admin products list --email seller@example.com --page 2 --per-page 25
  gumroad admin products list --email seller@example.com --json
  gumroad admin products view abc123
  gumroad admin products flag-for-tos-violation abc123 --user-id 2245593582708`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newFlagForTOSViolationCmd())

	return cmd
}
