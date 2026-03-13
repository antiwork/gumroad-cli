package subscribers

import (
	"github.com/spf13/cobra"
)

func NewSubscribersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subscribers",
		Short: "Manage product subscribers",
		Example: `  gumroad subscribers list --product <id>
  gumroad subscribers view <id>`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())

	return cmd
}
