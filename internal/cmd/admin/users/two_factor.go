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
	Email      string `json:"email,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Enabled    bool   `json:"enabled"`
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
  gumroad admin users two-factor disable --email user@example.com
  gumroad admin users two-factor disable --external-id 2245593582708`,
	}

	cmd.AddCommand(newTwoFactorEnableCmd())
	cmd.AddCommand(newTwoFactorDisableCmd())

	return cmd
}

func newTwoFactorEnableCmd() *cobra.Command {
	var (
		email      string
		externalID string
	)

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable two-factor authentication for a user",
		Long: `Identify the user with --email or --external-id. When both are supplied,
the server resolves by --external-id.`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			identifier := userIdentifier(email, externalID)
			return runTwoFactor(c, email, externalID, true, "Enable two-factor authentication for "+identifier+"?", "enable two-factor for "+identifier, "Enabling two-factor authentication...")
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "User external ID")

	return cmd
}

func newTwoFactorDisableCmd() *cobra.Command {
	var (
		email      string
		externalID string
	)

	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable two-factor authentication for a user",
		Long: `Disable two-factor authentication for a user. The user's existing TOTP
credential is destroyed; they will lose 2FA on their next login and any
recovery codes they had become invalid.

Identify the user with --email or --external-id. When both are supplied,
the server resolves by --external-id.`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			identifier := userIdentifier(email, externalID)
			return runTwoFactor(c, email, externalID, false, "Disable two-factor authentication for "+identifier+"? Their TOTP credential will be destroyed and they will lose 2FA on next login.", "disable two-factor for "+identifier, "Disabling two-factor authentication...")
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "User external ID")

	return cmd
}

func runTwoFactor(c *cobra.Command, email, externalID string, enabled bool, confirmMsg, cancelAction, spinnerMsg string) error {
	opts := cmdutil.OptionsFrom(c)
	if err := requireEmailOrExternalID(c, email, externalID); err != nil {
		return err
	}

	identifier := userIdentifier(email, externalID)
	ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
	if err != nil {
		return err
	}
	if !ok {
		return cmdutil.PrintCancelledAction(opts, cancelAction, identifier)
	}

	req := twoFactorRequest{Email: email, ExternalID: externalID, Enabled: enabled}

	if opts.DryRun {
		params := url.Values{}
		if email != "" {
			params.Set("email", email)
		}
		if externalID != "" {
			params.Set("external_id", externalID)
		}
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
	return renderTwoFactor(opts, identifier, decoded)
}

func renderTwoFactor(opts cmdutil.Options, identifier string, resp twoFactorResponse) error {
	state := "disabled"
	if resp.TwoFactorAuthenticationEnabled {
		state = "enabled"
	}
	message := resp.Message
	if message == "" {
		message = "Two-factor authentication " + state + " for " + identifier
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
