package purchases

import "github.com/spf13/cobra"

func NewPurchasesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purchases",
		Short: "Read and manage admin purchase records",
		Example: `  gumroad admin purchases view <purchase-id>
  gumroad admin purchases view <purchase-id> --json
  gumroad admin purchases refund <purchase-id> --email buyer@example.com`,
	}

	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newRefundCmd())

	return cmd
}
