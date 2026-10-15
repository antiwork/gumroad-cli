package stripe

import "github.com/spf13/cobra"

func NewStripeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stripe",
		Short: "Act on Stripe notices about sellers' connected accounts",
		Example: `  gumroad admin stripe court-order-notice --stripe-account-id acct_1Example --firm-name "Example Law LLP" --firm-email notices@example.com --notice-key notice-1 --dry-run
  gumroad admin stripe court-order-notice --stripe-account-id acct_1Example --firm-name "Example Law LLP" --firm-email notices@example.com --notice-key notice-1 --yes`,
	}

	cmd.AddCommand(newCourtOrderNoticeCmd())

	return cmd
}
