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

type updateEmailRequest struct {
	CurrentEmail string `json:"current_email,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	NewEmail     string `json:"new_email"`
}

type updateEmailResponse struct {
	Message             string `json:"message"`
	UnconfirmedEmail    string `json:"unconfirmed_email"`
	PendingConfirmation bool   `json:"pending_confirmation"`
}

func newUpdateEmailCmd() *cobra.Command {
	var (
		currentEmail string
		externalID   string
		newEmail     string
	)

	cmd := &cobra.Command{
		Use:   "update-email",
		Short: "Change a user's email address (pending user confirmation)",
		Long: `Stage a change to a user's email address. The new address is set as the
unconfirmed email and a confirmation message is sent to it; the user
must click the confirmation link before the new address takes effect.
Until then the existing email remains active.

Identify the user with --current-email or --external-id. When both are
supplied, the server resolves by --external-id.`,
		Example: `  gumroad admin users update-email --current-email old@example.com --new-email new@example.com
  gumroad admin users update-email --external-id 2245593582708 --new-email new@example.com
  gumroad admin users update-email --current-email old@example.com --new-email new@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if currentEmail == "" && externalID == "" {
				return cmdutil.UsageErrorf(c, "supply --current-email or --external-id")
			}
			if newEmail == "" {
				return cmdutil.MissingFlagError(c, "--new-email")
			}

			identifier := userIdentifier(currentEmail, externalID)
			ok, err := cmdutil.ConfirmAction(opts, "Change "+identifier+" to "+newEmail+"? The user must confirm via email before the change takes effect.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "update email from "+identifier+" to "+newEmail, identifier)
			}

			req := updateEmailRequest{CurrentEmail: currentEmail, ExternalID: externalID, NewEmail: newEmail}

			if opts.DryRun {
				params := url.Values{}
				if currentEmail != "" {
					params.Set("current_email", currentEmail)
				}
				if externalID != "" {
					params.Set("external_id", externalID)
				}
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
			return renderUpdateEmail(opts, identifier, newEmail, decoded)
		},
	}

	cmd.Flags().StringVar(&currentEmail, "current-email", "", "User's current email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "User external ID")
	cmd.Flags().StringVar(&newEmail, "new-email", "", "New email to stage (required)")

	return cmd
}

func renderUpdateEmail(opts cmdutil.Options, identifier, newEmail string, resp updateEmailResponse) error {
	unconfirmed := fallback(resp.UnconfirmedEmail, newEmail)
	defaultMessage := "Email change applied: " + identifier + " → " + unconfirmed
	if resp.PendingConfirmation {
		defaultMessage = "Email change pending confirmation: " + identifier + " → " + unconfirmed
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
	if err := output.Writef(opts.Out(), "Current: %s\n", identifier); err != nil {
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
