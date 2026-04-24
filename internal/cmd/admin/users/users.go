package users

import "github.com/spf13/cobra"

func NewUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Read admin user records",
		Example: `  gumroad admin users suspension --email user@example.com
  gumroad admin users suspension --email user@example.com --json`,
	}

	cmd.AddCommand(newSuspensionCmd())

	return cmd
}
