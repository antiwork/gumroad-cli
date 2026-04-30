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
	Email string `json:"email"`
}

type resetPasswordResponse struct {
	Message string `json:"message"`
}

func newResetPasswordCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: "Send password reset instructions to a user",
		Long: `Send Devise password reset instructions to a user. The email is delivered
to the address currently on file for the user, not to the admin.`,
		Example: `  gumroad admin users reset-password --email user@example.com
  gumroad admin users reset-password --email user@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Send password reset instructions to "+email+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "reset password for "+email, email)
			}

			req := resetPasswordRequest{Email: email}

			if opts.DryRun {
				params := url.Values{}
				params.Set("email", email)
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
			return renderResetPassword(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}

func renderResetPassword(opts cmdutil.Options, email string, resp resetPasswordResponse) error {
	message := fallback(resp.Message, "Reset password instructions sent to "+email)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, email}})
	}

	if opts.Quiet {
		return nil
	}

	return output.Writeln(opts.Out(), opts.Style().Green(message))
}
