package variants

import (
	"github.com/spf13/cobra"
)

func NewVariantsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variants",
		Short: "Manage variants",
		Example: `  gumroad variants list --product <id> --category <cat_id>
  gumroad variants create --product <id> --category <cat_id> --name "Large"
  gumroad variants update <variant_id> --product <id> --category <cat_id> --file ./license.pdf`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
