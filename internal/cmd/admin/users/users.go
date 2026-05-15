package users

import (
	"io"
	"strings"

	usercomments "github.com/antiwork/gumroad-cli/internal/cmd/admin/users/comments"
	"github.com/antiwork/gumroad-cli/internal/cmd/admin/users/usertarget"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func NewUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Read and manage admin user records",
		Example: `  gumroad admin users info --email user@example.com
  gumroad admin users info --user-id 2245593582708
  gumroad admin users affiliates --user-id 2245593582708 --direction granted
  gumroad admin users comments list --user-id 2245593582708
  gumroad admin users comments add --user-id 2245593582708 --content "VAT exempt confirmed"
  gumroad admin users compliance --user-id 2245593582708
  gumroad admin users radar --user-id 2245593582708
  gumroad admin users purchases --user-id 2245593582708 --status successful
  gumroad admin users related --email user@example.com --signal ip --signal payment_address
  gumroad admin users suspension --email user@example.com
  gumroad admin users mark-compliant --user-id 2245593582708 --expected-email user@example.com
  gumroad admin users watch --user-id 2245593582708 --revenue-threshold 200 --note "Review next buyers"
  gumroad admin users update-watch --user-id 2245593582708 --revenue-threshold 500
  gumroad admin users unwatch --user-id 2245593582708
  gumroad admin users suspend --user-id 2245593582708 --note "Chargeback risk confirmed"
  gumroad admin users reset-password --user-id 2245593582708
  gumroad admin users update-email --user-id 2245593582708 --new-email new@example.com
  gumroad admin users two-factor disable --user-id 2245593582708`,
	}

	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newAffiliatesCmd())
	cmd.AddCommand(usercomments.NewCommentsCmd())
	cmd.AddCommand(newComplianceCmd())
	cmd.AddCommand(newRadarCmd())
	cmd.AddCommand(newPurchasesCmd())
	cmd.AddCommand(newRelatedCmd())
	cmd.AddCommand(newSuspensionCmd())
	cmd.AddCommand(newMarkCompliantCmd())
	cmd.AddCommand(newWatchCmd())
	cmd.AddCommand(newUpdateWatchCmd())
	cmd.AddCommand(newUnwatchCmd())
	cmd.AddCommand(newSuspendCmd())
	cmd.AddCommand(newResetPasswordCmd())
	cmd.AddCommand(newUpdateEmailCmd())
	cmd.AddCommand(newTwoFactorCmd())

	return cmd
}

func fallback(value, alt string) string {
	return usertarget.Fallback(value, alt)
}

func writeIdentifierLine(w io.Writer, label, message, identifier string) error {
	if identifier == "" || strings.Contains(message, identifier) {
		return nil
	}
	return output.Writef(w, "%s: %s\n", label, identifier)
}

type userLookupFlags = usertarget.LookupFlags
type userLookupTarget = usertarget.LookupTarget

func addUserLookupFlags(cmd *cobra.Command, flags *userLookupFlags) {
	usertarget.AddLookupFlags(cmd, flags)
}

func resolveUserLookupTarget(cmd *cobra.Command, flags userLookupFlags) (userLookupTarget, error) {
	return usertarget.ResolveLookupTarget(cmd, flags)
}

type userMutationFlags = usertarget.MutationFlags
type userMutationTarget = usertarget.MutationTarget

func addUserMutationFlags(cmd *cobra.Command, flags *userMutationFlags) {
	usertarget.AddMutationFlags(cmd, flags)
}

func resolveUserMutationTarget(cmd *cobra.Command, flags userMutationFlags) (userMutationTarget, error) {
	return usertarget.ResolveMutationTarget(cmd, flags)
}
