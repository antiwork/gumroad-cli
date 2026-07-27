package users

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type markCompliantRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
	Note          string `json:"note,omitempty"`
	// ClearSuspension has to be sent for a suspended account. The server refuses to lift a
	// suspension without it, so an automated review can't reverse one by accident.
	ClearSuspension bool `json:"clear_suspension,omitempty"`
}

func newMarkCompliantCmd() *cobra.Command {
	var (
		targetFlags     userMutationFlags
		note            string
		clearSuspension bool
	)

	cmd := &cobra.Command{
		Use:   "mark-compliant",
		Short: "Mark a user compliant as an admin",
		Long: `Mark a user compliant through the internal admin API.

Identify the user with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.

If the account is suspended, pass --clear-suspension to also lift the suspension.
Without it the request is refused, so a review that only looked at its own signals
cannot silently put a suspended seller's products back on sale.`,
		Example: `  gumroad admin users mark-compliant --user-id 2245593582708
  gumroad admin users mark-compliant --user-id 2245593582708 --expected-email seller@example.com
  gumroad admin users mark-compliant --user-id 2245593582708 --note "Cleared after review"
  gumroad admin users mark-compliant --user-id 2245593582708 --clear-suspension --note "Suspension lifted after appeal"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			identifier := target.Identifier()
			prompt := "Mark user_id " + identifier + " compliant?"
			if clearSuspension {
				prompt = "Mark user_id " + identifier + " compliant and lift its suspension?"
			}
			ok, err := cmdutil.ConfirmAction(opts, prompt)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "mark user_id "+identifier+" compliant", identifier)
			}

			req := markCompliantRequest{
				UserID:          target.UserID,
				ExpectedEmail:   target.ExpectedEmail,
				Note:            note,
				ClearSuspension: clearSuspension,
			}
			path := "users/mark_compliant"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), markCompliantDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Marking user compliant...", path, req)
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
	cmd.Flags().StringVar(&note, "note", "", "Optional admin note")
	cmd.Flags().BoolVar(&clearSuspension, "clear-suspension", false, "Also lift the account's suspension (required when the account is suspended)")

	return cmd
}

func markCompliantDryRunParams(req markCompliantRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	if req.Note != "" {
		params.Set("note", req.Note)
	}
	if req.ClearSuspension {
		params.Set("clear_suspension", "true")
	}
	return params
}
