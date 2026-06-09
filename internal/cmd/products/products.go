package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmd/skus"
	"github.com/spf13/cobra"
)

func NewProductsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "products",
		Short: "Manage products",
		Long: "Manage products.\n\n" +
			"Create, update, list, view, delete, publish, and unpublish products. " +
			"New products are created as drafts; use `gumroad products publish <id>` to publish.",
		Example: `  gumroad products list
  gumroad products create --name "Art Pack" --price 10.00
  gumroad products create --name "Art Pack" --file ./pack.zip --file-name "Art Pack.zip"
  gumroad products create --name "Art Pack" --cover-image ./cover.jpg --thumbnail ./thumb.jpg
  gumroad products categories --search figma
  gumroad products create --name "Figma Kit" --category design/ui-and-web/figma
  gumroad products update <product_id> --name "New Name"
  gumroad products update <product_id> --category design/ui-and-web/figma
  gumroad products update <product_id> --preview-image ./gallery.jpg
  gumroad products page preview <product_id> ./landing.html
  gumroad products page publish <product_id> ./landing.html
  gumroad products covers add <product_id> --image ./cover.jpg
  gumroad products thumbnail set <product_id> --image ./thumb.jpg
  gumroad products update <product_id> --file ./pack.zip
  gumroad products update <product_id> --replace-files --keep-file file_123 --file ./new-pack.zip
  gumroad products content get <product_id> > content.json
  gumroad products content get <product_id> --variant <variant_id> --category <cat_id> > content.json
  gumroad products content set <product_id> content.json --dry-run
  gumroad products view <id>
  gumroad products url <id>
  gumroad products publish <id>
  gumroad products unpublish <id>
  gumroad products delete <id>
  gumroad products skus <id>`,
	}

	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newCategoriesCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newURLCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newPublishCmd())
	cmd.AddCommand(newUnpublishCmd())
	cmd.AddCommand(newPageCmd())
	cmd.AddCommand(newContentCmd())
	cmd.AddCommand(newCoversCmd())
	cmd.AddCommand(newThumbnailCmd())
	cmd.AddCommand(skus.NewProductSKUsCmd())

	return cmd
}
