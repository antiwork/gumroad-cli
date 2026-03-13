package licenses

import (
	"github.com/spf13/cobra"
)

func NewLicensesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "licenses",
		Short: "Manage product licenses",
		Example: `  echo "$LICENSE_KEY" | gumroad licenses verify --product <id> --no-increment
  echo "$LICENSE_KEY" | gumroad licenses enable --product <id>
  echo "$LICENSE_KEY" | gumroad licenses disable --product <id>`,
	}

	cmd.AddCommand(newVerifyCmd())
	cmd.AddCommand(newEnableCmd())
	cmd.AddCommand(newDisableCmd())
	cmd.AddCommand(newDecrementCmd())
	cmd.AddCommand(newRotateCmd())

	return cmd
}
