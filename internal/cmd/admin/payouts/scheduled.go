package payouts

import "github.com/spf13/cobra"

func newScheduledCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduled",
		Short: "Inspect and act on ScheduledPayout records",
		Example: `  gumroad admin payouts scheduled list
  gumroad admin payouts scheduled list --status flagged
  gumroad admin payouts scheduled execute pay_abc123 --yes
  gumroad admin payouts scheduled cancel pay_abc123`,
	}

	cmd.AddCommand(newScheduledListCmd())
	cmd.AddCommand(newScheduledExecuteCmd())
	cmd.AddCommand(newScheduledCancelCmd())

	return cmd
}

var validScheduledStatuses = map[string]struct{}{
	"pending":   {},
	"executed":  {},
	"cancelled": {},
	"flagged":   {},
	"held":      {},
}
