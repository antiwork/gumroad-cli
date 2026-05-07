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

type resetPasswordRequest struct {
	Email      string `json:"email,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

type resetPasswordResponse struct {
	Message string `json:"message"`
}

func newResetPasswordCmd() *cobra.Command {
	var (
		email      string
		externalID string
	)

	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: "Send password reset instructions to a user",
		Long: `Send Devise password reset instructions to a user. The email is delivered
to the address currently on file for the user, not to the admin.

Identify the user with --email or --external-id. When both are supplied, the
server resolves by --external-id.`,
		Example: `  gumroad admin users reset-password --email user@example.com
  gumroad admin users reset-password --external-id 2245593582708
  gumroad admin users reset-password --email user@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := requireEmailOrExternalID(c, email, externalID); err != nil {
				return err
			}

			identifier := userIdentifier(email, externalID)
			ok, err := cmdutil.ConfirmAction(opts, "Send password reset instructions to "+identifier+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "reset password for "+identifier, identifier)
			}

			req := resetPasswordRequest{Email: email, ExternalID: externalID}

			if opts.DryRun {
				params := url.Values{}
				if email != "" {
					params.Set("email", email)
				}
				if externalID != "" {
					params.Set("external_id", externalID)
				}
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
			return renderResetPassword(opts, identifier, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "User external ID")

	return cmd
}

func renderResetPassword(opts cmdutil.Options, identifier string, resp resetPasswordResponse) error {
	message := fallback(resp.Message, "Reset password instructions sent to "+identifier)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, identifier}})
	}

	if opts.Quiet {
		return nil
	}

	return output.Writeln(opts.Out(), opts.Style().Green(message))
}
