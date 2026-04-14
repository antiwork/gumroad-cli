package offercodes

import (
	"github.com/spf13/cobra"
)

func NewOfferCodesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "offer-codes",
		Aliases: []string{"oc"},
		Short:   "Manage offer codes",
		Example: `  gumroad offer-codes list --product <id>
  gumroad offer-codes create --product <id> --name SAVE10 --percent-off 10`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
