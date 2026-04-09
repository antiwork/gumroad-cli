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
			"Create, list, view, delete, publish, and unpublish products. " +
			"New products are created as drafts; use `gumroad products publish <id>` to publish.",
		Example: `  gumroad products list
  gumroad products create --name "Art Pack" --price 10.00
  gumroad products view <id>
  gumroad products publish <id>
  gumroad products unpublish <id>
  gumroad products delete <id>
  gumroad products skus <id>`,
	}

	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newPublishCmd())
	cmd.AddCommand(newUnpublishCmd())
	cmd.AddCommand(skus.NewProductSKUsCmd())

	return cmd
}
