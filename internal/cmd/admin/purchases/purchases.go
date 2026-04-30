package purchases

import "github.com/spf13/cobra"

func NewPurchasesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purchases",
		Short: "Read and manage admin purchase records",
		Example: `  gumroad admin purchases view <purchase-id>
  gumroad admin purchases search --email buyer@example.com
  gumroad admin purchases refund <purchase-id> --email buyer@example.com
  gumroad admin purchases refund-taxes <purchase-id> --email buyer@example.com
  gumroad admin purchases refund-for-fraud <purchase-id> --email buyer@example.com
  gumroad admin purchases cancel-subscription <purchase-id> --email buyer@example.com
  gumroad admin purchases block-buyer <purchase-id> --email buyer@example.com
  gumroad admin purchases unblock-buyer <purchase-id> --email buyer@example.com
  gumroad admin purchases resend-receipt <purchase-id>
  gumroad admin purchases resend-all-receipts --email buyer@example.com
  gumroad admin purchases reassign --from old@example.com --to new@example.com`,
	}

	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newRefundCmd())
	cmd.AddCommand(newRefundTaxesCmd())
	cmd.AddCommand(newRefundForFraudCmd())
	cmd.AddCommand(newCancelSubscriptionCmd())
	cmd.AddCommand(newBlockBuyerCmd())
	cmd.AddCommand(newUnblockBuyerCmd())
	cmd.AddCommand(newResendReceiptCmd())
	cmd.AddCommand(newResendAllReceiptsCmd())
	cmd.AddCommand(newReassignCmd())

	return cmd
}
