package comments

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

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

func (t userLookupTarget) values() url.Values {
	params := url.Values{}
	if t.Email != "" {
		params.Set("email", t.Email)
	}
	if t.UserID != "" {
		params.Set("user_id", t.UserID)
	}
	return params
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

func userMutationParams(target userMutationTarget) url.Values {
	params := url.Values{}
	params.Set("user_id", target.UserID)
	if target.ExpectedEmail != "" {
		params.Set("expected_email", target.ExpectedEmail)
	}
	return params
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
