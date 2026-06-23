package usertarget

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type LookupFlags struct {
	Email           string
	UserID          string
	Username        string
	ExternalIDAlias string
}

type LookupTarget struct {
	Email    string
	UserID   string
	Username string
}

func AddLookupFlags(cmd *cobra.Command, flags *LookupFlags) {
	cmd.Flags().StringVar(&flags.Email, "email", "", "User email")
	cmd.Flags().StringVar(&flags.UserID, "user-id", "", "User external ID")
	cmd.Flags().StringVar(&flags.Username, "username", "", "User username")
	cmd.Flags().StringVar(&flags.ExternalIDAlias, "external-id", "", "Alias for --user-id")
	_ = cmd.Flags().MarkHidden("external-id")
}

func ResolveLookupTarget(cmd *cobra.Command, flags LookupFlags) (LookupTarget, error) {
	userID, err := ResolveUserIDAlias(cmd, flags.UserID, flags.ExternalIDAlias)
	if err != nil {
		return LookupTarget{}, err
	}
	if err := RequireUserLookup(cmd, flags.Email, userID, flags.Username); err != nil {
		return LookupTarget{}, err
	}
	return LookupTarget{Email: flags.Email, UserID: userID, Username: flags.Username}, nil
}

func (t LookupTarget) Identifier() string {
	return UserIdentifier(t.Email, t.UserID, t.Username)
}

func (t LookupTarget) Values() url.Values {
	params := url.Values{}
	if t.Email != "" {
		params.Set("email", t.Email)
	}
	if t.UserID != "" {
		params.Set("user_id", t.UserID)
	}
	if t.Username != "" {
		params.Set("username", t.Username)
	}
	return params
}

type MutationFlags struct {
	UserID             string
	ExternalIDAlias    string
	ExpectedEmail      string
	ExpectedEmailAlias string
}

type MutationTarget struct {
	UserID        string
	ExpectedEmail string
}

func AddMutationFlags(cmd *cobra.Command, flags *MutationFlags) {
	cmd.Flags().StringVar(&flags.UserID, "user-id", "", "User external ID (required)")
	cmd.Flags().StringVar(&flags.ExpectedEmail, "expected-email", "", "Optional current email guard")
	cmd.Flags().StringVar(&flags.ExternalIDAlias, "external-id", "", "Alias for --user-id")
	cmd.Flags().StringVar(&flags.ExpectedEmailAlias, "email", "", "Alias for --expected-email")
	_ = cmd.Flags().MarkHidden("external-id")
	_ = cmd.Flags().MarkHidden("email")
}

func ResolveMutationTarget(cmd *cobra.Command, flags MutationFlags) (MutationTarget, error) {
	userID, err := ResolveUserIDAlias(cmd, flags.UserID, flags.ExternalIDAlias)
	if err != nil {
		return MutationTarget{}, err
	}
	expectedEmail, err := ResolveExpectedEmailAlias(cmd, flags.ExpectedEmail, flags.ExpectedEmailAlias)
	if err != nil {
		return MutationTarget{}, err
	}
	if userID == "" {
		return MutationTarget{}, cmdutil.MissingFlagError(cmd, "--user-id")
	}
	return MutationTarget{UserID: userID, ExpectedEmail: expectedEmail}, nil
}

func (t MutationTarget) Identifier() string {
	return t.UserID
}

func (t MutationTarget) Subject() string {
	return "user_id " + t.UserID
}

func MutationParams(target MutationTarget) url.Values {
	params := url.Values{}
	params.Set("user_id", target.UserID)
	if target.ExpectedEmail != "" {
		params.Set("expected_email", target.ExpectedEmail)
	}
	return params
}

func Fallback(value, alt string) string {
	if value == "" {
		return alt
	}
	return value
}

func UserIdentifier(email, externalID, username string) string {
	if externalID != "" {
		return externalID
	}
	if email != "" {
		return email
	}
	return username
}

func RequireUserLookup(cmd *cobra.Command, email, userID, username string) error {
	if email == "" && userID == "" && username == "" {
		return cmdutil.UsageErrorf(cmd, "supply --email, --user-id, or --username")
	}
	return nil
}

func ResolveUserIDAlias(cmd *cobra.Command, userID, externalIDAlias string) (string, error) {
	if userID != "" && externalIDAlias != "" && userID != externalIDAlias {
		return "", cmdutil.UsageErrorf(cmd, "--user-id and --external-id must match")
	}
	if userID != "" {
		return userID, nil
	}
	return externalIDAlias, nil
}

func ResolveExpectedEmailAlias(cmd *cobra.Command, expectedEmail, emailAlias string) (string, error) {
	if expectedEmail != "" && emailAlias != "" && expectedEmail != emailAlias {
		return "", cmdutil.UsageErrorf(cmd, "--expected-email and --email must match")
	}
	if expectedEmail != "" {
		return expectedEmail, nil
	}
	return emailAlias, nil
}
