package payouts

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type lookupFlags struct {
	Email           string
	UserID          string
	ExternalIDAlias string
}

type lookupTarget struct {
	Email  string
	UserID string
}

func addLookupFlags(cmd *cobra.Command, flags *lookupFlags) {
	cmd.Flags().StringVar(&flags.Email, "email", "", "User email")
	cmd.Flags().StringVar(&flags.UserID, "user-id", "", "User external ID")
	cmd.Flags().StringVar(&flags.ExternalIDAlias, "external-id", "", "Alias for --user-id")
	_ = cmd.Flags().MarkHidden("external-id")
}

func resolveLookupTarget(cmd *cobra.Command, flags lookupFlags) (lookupTarget, error) {
	userID, err := resolveUserIDAlias(cmd, flags.UserID, flags.ExternalIDAlias)
	if err != nil {
		return lookupTarget{}, err
	}
	if flags.Email == "" && userID == "" {
		return lookupTarget{}, cmdutil.UsageErrorf(cmd, "supply --email or --user-id")
	}
	return lookupTarget{Email: flags.Email, UserID: userID}, nil
}

func (t lookupTarget) identifier() string {
	if t.UserID != "" {
		return t.UserID
	}
	return t.Email
}

type mutationFlags struct {
	UserID             string
	ExternalIDAlias    string
	ExpectedEmail      string
	ExpectedEmailAlias string
}

type mutationTarget struct {
	UserID        string
	ExpectedEmail string
}

func addMutationFlags(cmd *cobra.Command, flags *mutationFlags) {
	cmd.Flags().StringVar(&flags.UserID, "user-id", "", "User external ID (required)")
	cmd.Flags().StringVar(&flags.ExpectedEmail, "expected-email", "", "Optional current email guard")
	cmd.Flags().StringVar(&flags.ExternalIDAlias, "external-id", "", "Alias for --user-id")
	cmd.Flags().StringVar(&flags.ExpectedEmailAlias, "email", "", "Alias for --expected-email")
	_ = cmd.Flags().MarkHidden("external-id")
	_ = cmd.Flags().MarkHidden("email")
}

func resolveMutationTarget(cmd *cobra.Command, flags mutationFlags) (mutationTarget, error) {
	userID, err := resolveUserIDAlias(cmd, flags.UserID, flags.ExternalIDAlias)
	if err != nil {
		return mutationTarget{}, err
	}
	expectedEmail, err := resolveExpectedEmailAlias(cmd, flags.ExpectedEmail, flags.ExpectedEmailAlias)
	if err != nil {
		return mutationTarget{}, err
	}
	if userID == "" {
		return mutationTarget{}, cmdutil.MissingFlagError(cmd, "--user-id")
	}
	return mutationTarget{UserID: userID, ExpectedEmail: expectedEmail}, nil
}

func mutationParams(target mutationTarget) url.Values {
	params := url.Values{}
	params.Set("user_id", target.UserID)
	if target.ExpectedEmail != "" {
		params.Set("expected_email", target.ExpectedEmail)
	}
	return params
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
