package users

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func NewUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Read and manage admin user records",
		Example: `  gumroad admin users info --email user@example.com
  gumroad admin users info --external-id 2245593582708
  gumroad admin users suspension --email user@example.com
  gumroad admin users mark-compliant --email user@example.com
  gumroad admin users watch --email user@example.com --revenue-threshold 200 --note "Review next buyers"
  gumroad admin users update-watch --email user@example.com --revenue-threshold 500
  gumroad admin users unwatch --email user@example.com
  gumroad admin users suspend --email user@example.com --note "Chargeback risk confirmed"
  gumroad admin users reset-password --email user@example.com
  gumroad admin users update-email --current-email old@example.com --new-email new@example.com
  gumroad admin users two-factor disable --email user@example.com
  gumroad admin users add-comment --email user@example.com --content "VAT exempt confirmed"`,
	}

	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newSuspensionCmd())
	cmd.AddCommand(newMarkCompliantCmd())
	cmd.AddCommand(newWatchCmd())
	cmd.AddCommand(newUpdateWatchCmd())
	cmd.AddCommand(newUnwatchCmd())
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

func userIdentifier(email, externalID string) string {
	if email != "" {
		return email
	}
	return externalID
}

func requireEmailOrExternalID(cmd *cobra.Command, email, externalID string) error {
	if email == "" && externalID == "" {
		return cmdutil.UsageErrorf(cmd, "supply --email or --external-id")
	}
	return nil
}
