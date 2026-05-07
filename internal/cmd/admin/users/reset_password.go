package users

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type resetPasswordRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

type resetPasswordResponse struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

func newResetPasswordCmd() *cobra.Command {
	var targetFlags userMutationFlags

	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: "Send password reset instructions to a user",
		Long: `Send Devise password reset instructions to a user. The email is delivered
to the address currently on file for the user, not to the admin.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.`,
		Example: `  gumroad admin users reset-password --user-id 2245593582708
  gumroad admin users reset-password --user-id 2245593582708 --expected-email user@example.com
  gumroad admin users reset-password --user-id 2245593582708 --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			identifier := target.identifier()
			ok, err := cmdutil.ConfirmAction(opts, "Send password reset instructions to user_id "+identifier+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "reset password for user_id "+identifier, identifier)
			}

			req := resetPasswordRequest(target)

			if opts.DryRun {
				params := userMutationParams(target)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath("/users/reset_password"), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Sending reset instructions...", "/users/reset_password", req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[resetPasswordResponse](data)
			if err != nil {
				return err
			}
			return renderResetPassword(opts, fallback(decoded.UserID, identifier), decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)

	return cmd
}

func renderResetPassword(opts cmdutil.Options, identifier string, resp resetPasswordResponse) error {
	message := fallback(resp.Message, "Reset password instructions sent to user_id "+identifier)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, identifier}})
	}

	if opts.Quiet {
		return nil
	}

	return output.Writeln(opts.Out(), opts.Style().Green(message))
}
