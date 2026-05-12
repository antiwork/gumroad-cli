package users

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type twoFactorRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
	Enabled       bool   `json:"enabled"`
}

type twoFactorResponse struct {
	UserID                         string `json:"user_id"`
	Message                        string `json:"message"`
	TwoFactorAuthenticationEnabled bool   `json:"two_factor_authentication_enabled"`
}

func newTwoFactorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "two-factor",
		Short: "Enable or disable two-factor authentication for a user",
		Example: `  gumroad admin users two-factor enable --user-id 2245593582708
  gumroad admin users two-factor disable --user-id 2245593582708
  gumroad admin users two-factor disable --user-id 2245593582708 --expected-email user@example.com`,
	}

	cmd.AddCommand(newTwoFactorEnableCmd())
	cmd.AddCommand(newTwoFactorDisableCmd())

	return cmd
}

func newTwoFactorEnableCmd() *cobra.Command {
	var targetFlags userMutationFlags

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable two-factor authentication for a user",
		Long: `Enable two-factor authentication for a user.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			return runTwoFactor(c, targetFlags, true, "enable", "Enabling two-factor authentication...")
		},
	}

	addUserMutationFlags(cmd, &targetFlags)

	return cmd
}

func newTwoFactorDisableCmd() *cobra.Command {
	var targetFlags userMutationFlags

	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable two-factor authentication for a user",
		Long: `Disable two-factor authentication for a user. The user's existing TOTP
credential is destroyed; they will lose 2FA on their next login and any
recovery codes they had become invalid.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			return runTwoFactor(c, targetFlags, false, "disable", "Disabling two-factor authentication...")
		},
	}

	addUserMutationFlags(cmd, &targetFlags)

	return cmd
}

func runTwoFactor(c *cobra.Command, flags userMutationFlags, enabled bool, verb, spinnerMsg string) error {
	opts := cmdutil.OptionsFrom(c)
	target, err := resolveUserMutationTarget(c, flags)
	if err != nil {
		return err
	}

	identifier := target.Identifier()
	confirmMsg := "Enable two-factor authentication for user_id " + identifier + "?"
	cancelAction := "enable two-factor for user_id " + identifier
	if verb == "disable" {
		confirmMsg = "Disable two-factor authentication for user_id " + identifier + "? Their TOTP credential will be destroyed and they will lose 2FA on next login."
		cancelAction = "disable two-factor for user_id " + identifier
	}
	ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
	if err != nil {
		return err
	}
	if !ok {
		return cmdutil.PrintCancelledAction(opts, cancelAction, identifier)
	}

	req := twoFactorRequest{UserID: target.UserID, ExpectedEmail: target.ExpectedEmail, Enabled: enabled}

	if opts.DryRun {
		params := userMutationParams(target)
		if enabled {
			params.Set("enabled", "true")
		} else {
			params.Set("enabled", "false")
		}
		return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath("/users/two_factor_authentication"), params)
	}

	data, err := admincmd.FetchPostJSON(opts, spinnerMsg, "/users/two_factor_authentication", req)
	if err != nil {
		return err
	}

	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}

	decoded, err := cmdutil.DecodeJSON[twoFactorResponse](data)
	if err != nil {
		return err
	}
	return renderTwoFactor(opts, fallback(decoded.UserID, identifier), decoded)
}

func renderTwoFactor(opts cmdutil.Options, identifier string, resp twoFactorResponse) error {
	state := "disabled"
	if resp.TwoFactorAuthenticationEnabled {
		state = "enabled"
	}
	message := resp.Message
	if message == "" {
		message = "Two-factor authentication " + state + " for user_id " + identifier
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, identifier, state}})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Two-factor: %s\n", state)
}
