package variants

import (
	"github.com/spf13/cobra"
)

func NewVariantsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variants",
		Short: "Manage variants",
		Example: `  gumroad variants list --product <id> --category <cat_id>
  gumroad variants create --product <id> --category <cat_id> --name "Large"`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
