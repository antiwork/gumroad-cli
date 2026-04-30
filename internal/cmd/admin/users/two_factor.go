package users

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type twoFactorRequest struct {
	Email   string `json:"email"`
	Enabled bool   `json:"enabled"`
}

type twoFactorResponse struct {
	Message                        string `json:"message"`
	TwoFactorAuthenticationEnabled bool   `json:"two_factor_authentication_enabled"`
}

func newTwoFactorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "two-factor",
		Short: "Enable or disable two-factor authentication for a user",
		Example: `  gumroad admin users two-factor enable --email user@example.com
  gumroad admin users two-factor disable --email user@example.com`,
	}

	cmd.AddCommand(newTwoFactorEnableCmd())
	cmd.AddCommand(newTwoFactorDisableCmd())

	return cmd
}

func newTwoFactorEnableCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable two-factor authentication for a user",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			return runTwoFactor(c, email, true, "Enable two-factor authentication for "+email+"?", "enable two-factor for "+email, "Enabling two-factor authentication...")
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}

func newTwoFactorDisableCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable two-factor authentication for a user",
		Long: `Disable two-factor authentication for a user. The user's existing TOTP
credential is destroyed; they will lose 2FA on their next login and any
recovery codes they had become invalid.`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			return runTwoFactor(c, email, false, "Disable two-factor authentication for "+email+"? Their TOTP credential will be destroyed and they will lose 2FA on next login.", "disable two-factor for "+email, "Disabling two-factor authentication...")
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}

func runTwoFactor(c *cobra.Command, email string, enabled bool, confirmMsg, cancelAction, spinnerMsg string) error {
	opts := cmdutil.OptionsFrom(c)
	if email == "" {
		return cmdutil.MissingFlagError(c, "--email")
	}

	ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
	if err != nil {
		return err
	}
	if !ok {
		return cmdutil.PrintCancelledAction(opts, cancelAction, email)
	}

	req := twoFactorRequest{Email: email, Enabled: enabled}

	if opts.DryRun {
		params := url.Values{}
		params.Set("email", email)
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
	return renderTwoFactor(opts, email, decoded)
}

func renderTwoFactor(opts cmdutil.Options, email string, resp twoFactorResponse) error {
	state := "disabled"
	if resp.TwoFactorAuthenticationEnabled {
		state = "enabled"
	}
	message := resp.Message
	if message == "" {
		message = "Two-factor authentication " + state + " for " + email
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, email, state}})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Two-factor: %s\n", state)
}
