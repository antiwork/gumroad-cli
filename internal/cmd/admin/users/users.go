package users

import "github.com/spf13/cobra"

func NewUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Read and update admin user records",
		Example: `  gumroad admin users suspension --email user@example.com
  gumroad admin users suspension --email user@example.com --json
  gumroad admin users mark-compliant --email user@example.com
  gumroad admin users suspend --email user@example.com --note "Chargeback risk confirmed"`,
	}

	cmd.AddCommand(newSuspensionCmd())
	cmd.AddCommand(newMarkCompliantCmd())
	cmd.AddCommand(newSuspendCmd())

	return cmd
}
