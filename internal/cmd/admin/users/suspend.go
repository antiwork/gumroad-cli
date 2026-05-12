package users

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

const suspendConfirmationMessage = "Suspend user_id %s for fraud? This freezes payouts and disables the seller's products."

type suspendRequest struct {
	UserID         string `json:"user_id"`
	ExpectedEmail  string `json:"expected_email,omitempty"`
	SuspensionNote string `json:"suspension_note,omitempty"`
}

func newSuspendCmd() *cobra.Command {
	var (
		targetFlags userMutationFlags
		note        string
	)

	cmd := &cobra.Command{
		Use:   "suspend",
		Short: "Suspend a user for fraud as an admin",
		Long: `Suspend a user for fraud through the internal admin API.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.`,
		Example: `  gumroad admin users suspend --user-id 2245593582708
  gumroad admin users suspend --user-id 2245593582708 --expected-email seller@example.com
  gumroad admin users suspend --user-id 2245593582708 --note "Chargeback risk confirmed"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			identifier := target.Identifier()
			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(suspendConfirmationMessage, identifier))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "suspend user_id "+identifier+" for fraud", identifier)
			}

			req := suspendRequest{
				UserID:         target.UserID,
				ExpectedEmail:  target.ExpectedEmail,
				SuspensionNote: note,
			}
			path := "users/suspend_for_fraud"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), suspendDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Suspending user...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[riskActionResponse](data)
			if err != nil {
				return err
			}
			return renderRiskAction(opts, "User ID", fallback(decoded.UserID, identifier), decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().StringVar(&note, "note", "", "Optional suspension note")

	return cmd
}

func suspendDryRunParams(req suspendRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	if req.SuspensionNote != "" {
		params.Set("suspension_note", req.SuspensionNote)
	}
	return params
}
