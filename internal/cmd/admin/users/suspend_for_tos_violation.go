package users

import (
	"fmt"
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

const suspendForTOSViolationConfirmationMessage = "Suspend user_id %s for a policy violation? This freezes payouts and disables the seller's products, but does not block them from buying as a customer."

func newSuspendForTOSViolationCmd() *cobra.Command {
	var (
		targetFlags userMutationFlags
		note        string
	)

	cmd := &cobra.Command{
		Use:   "suspend-for-tos-violation",
		Short: "Suspend a user for a policy violation as an admin",
		Long: `Suspend a user for a policy violation through the internal admin API.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.

This is distinct from fraud suspension: payouts and seller products are stopped,
but the user is not blocked from buying as a customer.`,
		Example: `  gumroad admin users suspend-for-tos-violation --user-id 2245593582708
  gumroad admin users suspend-for-tos-violation --user-id 2245593582708 --expected-email seller@example.com
  gumroad admin users suspend-for-tos-violation --user-id 2245593582708 --note "DMCA takedown notice confirmed"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			identifier := target.Identifier()
			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(suspendForTOSViolationConfirmationMessage, identifier))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "suspend user_id "+identifier+" for a policy violation", identifier)
			}

			req := suspendRequest{
				UserID:         target.UserID,
				ExpectedEmail:  target.ExpectedEmail,
				SuspensionNote: note,
			}
			path := "users/suspend_for_tos_violation"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), suspendDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Suspending user for policy violation...", path, req)
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
