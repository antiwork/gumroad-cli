package payouts

import "github.com/spf13/cobra"

func NewPayoutsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payouts",
		Short: "Read admin payout records",
		Example: `  gumroad admin payouts list --email seller@example.com
  gumroad admin payouts list --email seller@example.com --json`,
	}

	cmd.AddCommand(newListCmd())

	return cmd
}
