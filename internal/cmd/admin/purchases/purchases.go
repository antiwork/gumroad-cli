package purchases

import "github.com/spf13/cobra"

func NewPurchasesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purchases",
		Short: "Read admin purchase records",
		Example: `  gumroad admin purchases view <purchase-id>
  gumroad admin purchases view <purchase-id> --json`,
	}

	cmd.AddCommand(newViewCmd())

	return cmd
}
