package categories

import (
	"github.com/spf13/cobra"
)

func NewCategoriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "variant-categories",
		Aliases: []string{"vc"},
		Short:   "Manage product variant categories",
		Example: `  gumroad variant-categories list --product <id>
  gumroad variant-categories create --product <id> --title "Size"`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
