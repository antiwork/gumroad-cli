package payouts

import "github.com/spf13/cobra"

func NewPayoutsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payouts",
		Short: "Read and manage admin payout records",
		Example: `  gumroad admin payouts list --email seller@example.com
  gumroad admin payouts list --email seller@example.com --json
  gumroad admin payouts pause --email seller@example.com --reason "Verification pending"
  gumroad admin payouts resume --email seller@example.com`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newPauseCmd())
	cmd.AddCommand(newResumeCmd())

	return cmd
}
