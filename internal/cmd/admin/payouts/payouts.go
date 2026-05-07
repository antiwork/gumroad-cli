package payouts

import "github.com/spf13/cobra"

func NewPayoutsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payouts",
		Short: "Read and manage admin payout records",
		Example: `  gumroad admin payouts list --email seller@example.com
  gumroad admin payouts list --user-id 2245593582708
  gumroad admin payouts list --email seller@example.com --json
  gumroad admin payouts pause --user-id 2245593582708 --expected-email seller@example.com --reason "Verification pending"
  gumroad admin payouts resume --user-id 2245593582708
  gumroad admin payouts issue --user-id 2245593582708 --through 2026-04-30 --processor stripe --yes
  gumroad admin payouts scheduled list --status flagged`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newPauseCmd())
	cmd.AddCommand(newResumeCmd())
	cmd.AddCommand(newIssueCmd())
	cmd.AddCommand(newScheduledCmd())

	return cmd
}
