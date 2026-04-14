package customfields

import (
	"github.com/spf13/cobra"
)

func NewCustomFieldsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "custom-fields",
		Aliases: []string{"cf"},
		Short:   "Manage custom fields",
		Long:    "Manage custom fields for a product. Update and delete key by name, not ID.",
		Example: `  gumroad custom-fields list --product <id>
  gumroad custom-fields create --product <id> --name "Company" --required`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
