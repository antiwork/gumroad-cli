package users

import "github.com/spf13/cobra"

func NewUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Read and manage admin user records",
		Example: `  gumroad admin users suspension --email user@example.com
  gumroad admin users mark-compliant --email user@example.com
  gumroad admin users suspend --email user@example.com --note "Chargeback risk confirmed"
  gumroad admin users reset-password --email user@example.com
  gumroad admin users update-email --current-email old@example.com --new-email new@example.com
  gumroad admin users two-factor disable --email user@example.com
  gumroad admin users add-comment --email user@example.com --content "VAT exempt confirmed"`,
	}

	cmd.AddCommand(newSuspensionCmd())
	cmd.AddCommand(newMarkCompliantCmd())
	cmd.AddCommand(newSuspendCmd())
	cmd.AddCommand(newResetPasswordCmd())
	cmd.AddCommand(newUpdateEmailCmd())
	cmd.AddCommand(newTwoFactorCmd())
	cmd.AddCommand(newAddCommentCmd())

	return cmd
}

func fallback(value, alt string) string {
	if value == "" {
		return alt
	}
	return value
}
