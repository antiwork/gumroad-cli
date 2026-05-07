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
  gumroad admin users info --user-id 2245593582708
  gumroad admin users suspension --email user@example.com
  gumroad admin users mark-compliant --user-id 2245593582708 --expected-email user@example.com
  gumroad admin users watch --user-id 2245593582708 --revenue-threshold 200 --note "Review next buyers"
  gumroad admin users update-watch --user-id 2245593582708 --revenue-threshold 500
  gumroad admin users unwatch --user-id 2245593582708
  gumroad admin users suspend --user-id 2245593582708 --note "Chargeback risk confirmed"
  gumroad admin users reset-password --user-id 2245593582708
  gumroad admin users update-email --user-id 2245593582708 --new-email new@example.com
  gumroad admin users two-factor disable --user-id 2245593582708
  gumroad admin users add-comment --user-id 2245593582708 --content "VAT exempt confirmed"`,
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
	if externalID != "" {
		return externalID
	}
	return email
}

func requireEmailOrUserID(cmd *cobra.Command, email, userID string) error {
	if email == "" && userID == "" {
		return cmdutil.UsageErrorf(cmd, "supply --email or --user-id")
	}
	return nil
}

type userLookupFlags struct {
	Email           string
	UserID          string
	ExternalIDAlias string
}

type userLookupTarget struct {
	Email  string
	UserID string
}

func addUserLookupFlags(cmd *cobra.Command, flags *userLookupFlags) {
	cmd.Flags().StringVar(&flags.Email, "email", "", "User email")
	cmd.Flags().StringVar(&flags.UserID, "user-id", "", "User external ID")
	cmd.Flags().StringVar(&flags.ExternalIDAlias, "external-id", "", "Alias for --user-id")
	_ = cmd.Flags().MarkHidden("external-id")
}

func resolveUserLookupTarget(cmd *cobra.Command, flags userLookupFlags) (userLookupTarget, error) {
	userID, err := resolveUserIDAlias(cmd, flags.UserID, flags.ExternalIDAlias)
	if err != nil {
		return userLookupTarget{}, err
	}
	if err := requireEmailOrUserID(cmd, flags.Email, userID); err != nil {
		return userLookupTarget{}, err
	}
	return userLookupTarget{Email: flags.Email, UserID: userID}, nil
}

func (t userLookupTarget) identifier() string {
	return userIdentifier(t.Email, t.UserID)
}

type userMutationFlags struct {
	UserID             string
	ExternalIDAlias    string
	ExpectedEmail      string
	ExpectedEmailAlias string
}

type userMutationTarget struct {
	UserID        string
	ExpectedEmail string
}

func addUserMutationFlags(cmd *cobra.Command, flags *userMutationFlags) {
	cmd.Flags().StringVar(&flags.UserID, "user-id", "", "User external ID (required)")
	cmd.Flags().StringVar(&flags.ExpectedEmail, "expected-email", "", "Optional current email guard")
	cmd.Flags().StringVar(&flags.ExternalIDAlias, "external-id", "", "Alias for --user-id")
	cmd.Flags().StringVar(&flags.ExpectedEmailAlias, "email", "", "Alias for --expected-email")
	_ = cmd.Flags().MarkHidden("external-id")
	_ = cmd.Flags().MarkHidden("email")
}

func resolveUserMutationTarget(cmd *cobra.Command, flags userMutationFlags) (userMutationTarget, error) {
	userID, err := resolveUserIDAlias(cmd, flags.UserID, flags.ExternalIDAlias)
	if err != nil {
		return userMutationTarget{}, err
	}
	expectedEmail, err := resolveExpectedEmailAlias(cmd, flags.ExpectedEmail, flags.ExpectedEmailAlias)
	if err != nil {
		return userMutationTarget{}, err
	}
	if userID == "" {
		return userMutationTarget{}, cmdutil.MissingFlagError(cmd, "--user-id")
	}
	return userMutationTarget{UserID: userID, ExpectedEmail: expectedEmail}, nil
}

func (t userMutationTarget) identifier() string {
	return t.UserID
}

func (t userMutationTarget) subject() string {
	return "user_id " + t.UserID
}

func resolveUserIDAlias(cmd *cobra.Command, userID, externalIDAlias string) (string, error) {
	if userID != "" && externalIDAlias != "" && userID != externalIDAlias {
		return "", cmdutil.UsageErrorf(cmd, "--user-id and --external-id must match")
	}
	if userID != "" {
		return userID, nil
	}
	return externalIDAlias, nil
}

func resolveExpectedEmailAlias(cmd *cobra.Command, expectedEmail, emailAlias string) (string, error) {
	if expectedEmail != "" && emailAlias != "" && expectedEmail != emailAlias {
		return "", cmdutil.UsageErrorf(cmd, "--expected-email and --email must match")
	}
	if expectedEmail != "" {
		return expectedEmail, nil
	}
	return emailAlias, nil
}
