package sales

import (
	"github.com/spf13/cobra"
)

func NewSalesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sales",
		Short: "Manage sales",
		Example: `  gumroad sales list
  gumroad sales export --from 2026-01-01 --to 2026-05-21
  gumroad sales view <id>
  gumroad sales refund <id>`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newRefundCmd())
	cmd.AddCommand(newShipCmd())
	cmd.AddCommand(newResendReceiptCmd())

	return cmd
}
