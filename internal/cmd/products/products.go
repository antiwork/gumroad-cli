package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmd/skus"
	"github.com/spf13/cobra"
)

func NewProductsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "products",
		Short: "Manage your Gumroad products",
		Long: "Manage your Gumroad products.\n\n" +
			"Create, list, view, delete, enable, and disable products. " +
			"New products are created as drafts; use `gumroad products enable <id>` to publish.",
		Example: `  gumroad products list
  gumroad products create --name "Art Pack" --price 10.00
  gumroad products view <id>
  gumroad products delete <id>
  gumroad products skus <id>`,
	}

	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newEnableCmd())
	cmd.AddCommand(newDisableCmd())
	cmd.AddCommand(skus.NewProductSKUsCmd())

	return cmd
}
