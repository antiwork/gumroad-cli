package payouts

import (
	"github.com/spf13/cobra"
)

func NewPayoutsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payouts",
		Short: "View payouts",
		Example: `  gumroad payouts list
  gumroad payouts view <id>
  gumroad payouts upcoming`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newUpcomingCmd())

	return cmd
}
