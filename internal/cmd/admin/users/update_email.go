package users

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type updateEmailRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
	NewEmail      string `json:"new_email"`
}

type updateEmailResponse struct {
	UserID              string `json:"user_id"`
	Message             string `json:"message"`
	UnconfirmedEmail    string `json:"unconfirmed_email"`
	PendingConfirmation bool   `json:"pending_confirmation"`
}

func newUpdateEmailCmd() *cobra.Command {
	var (
		targetFlags       userMutationFlags
		currentEmailAlias string
		newEmail          string
	)

	cmd := &cobra.Command{
		Use:   "update-email",
		Short: "Change a user's email address (pending user confirmation)",
		Long: `Stage a change to a user's email address. The new address is set as the
unconfirmed email and a confirmation message is sent to it; the user
must click the confirmation link before the new address takes effect.
Until then the existing email remains active.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.`,
		Example: `  gumroad admin users update-email --user-id 2245593582708 --new-email new@example.com
  gumroad admin users update-email --user-id 2245593582708 --expected-email old@example.com --new-email new@example.com
  gumroad admin users update-email --user-id 2245593582708 --new-email new@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if currentEmailAlias != "" {
				if targetFlags.ExpectedEmail != "" && targetFlags.ExpectedEmail != currentEmailAlias {
					return cmdutil.UsageErrorf(c, "--expected-email and --current-email must match")
				}
				targetFlags.ExpectedEmail = currentEmailAlias
			}
			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}
			if newEmail == "" {
				return cmdutil.MissingFlagError(c, "--new-email")
			}

			identifier := target.identifier()
			confirmSubject := target.subject()
			ok, err := cmdutil.ConfirmAction(opts, "Change "+confirmSubject+" to "+newEmail+"? The user must confirm via email before the change takes effect.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "update email from "+confirmSubject+" to "+newEmail, identifier)
			}

			req := updateEmailRequest{UserID: target.UserID, ExpectedEmail: target.ExpectedEmail, NewEmail: newEmail}

			if opts.DryRun {
				params := userMutationParams(target)
				params.Set("new_email", newEmail)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath("/users/update_email"), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Updating user email...", "/users/update_email", req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[updateEmailResponse](data)
			if err != nil {
				return err
			}
			return renderUpdateEmail(opts, fallback(decoded.UserID, identifier), newEmail, decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().StringVar(&currentEmailAlias, "current-email", "", "Alias for --expected-email")
	_ = cmd.Flags().MarkHidden("current-email")
	cmd.Flags().StringVar(&newEmail, "new-email", "", "New email to stage (required)")

	return cmd
}

func renderUpdateEmail(opts cmdutil.Options, identifier, newEmail string, resp updateEmailResponse) error {
	unconfirmed := fallback(resp.UnconfirmedEmail, newEmail)
	subject := "user_id " + identifier
	defaultMessage := "Email change applied: " + subject + " → " + unconfirmed
	if resp.PendingConfirmation {
		defaultMessage = "Email change pending confirmation: " + subject + " → " + unconfirmed
	}
	message := fallback(resp.Message, defaultMessage)
	pending := "false"
	if resp.PendingConfirmation {
		pending = "true"
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, identifier, unconfirmed, pending}})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "User ID: %s\n", identifier); err != nil {
		return err
	}
	if resp.PendingConfirmation {
		if err := output.Writef(opts.Out(), "Pending: %s\n", unconfirmed); err != nil {
			return err
		}
	}
	return output.Writef(opts.Out(), "Confirmed by user: %s\n", boolLabel(!resp.PendingConfirmation))
}

func boolLabel(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
