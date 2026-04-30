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

const suspendConfirmationMessage = "Suspend user %s for fraud? This freezes payouts and disables the seller's products."

type suspendRequest struct {
	Email          string `json:"email"`
	SuspensionNote string `json:"suspension_note,omitempty"`
}

func newSuspendCmd() *cobra.Command {
	var (
		email string
		note  string
	)

	cmd := &cobra.Command{
		Use:   "suspend",
		Short: "Suspend a user for fraud as an admin",
		Long:  "Suspend a user for fraud through the internal admin API.",
		Example: `  gumroad admin users suspend --email seller@example.com
  gumroad admin users suspend --email seller@example.com --note "Chargeback risk confirmed"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(suspendConfirmationMessage, email))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "suspend user "+email+" for fraud", email)
			}

			req := suspendRequest{
				Email:          email,
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
			return renderRiskAction(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&note, "note", "", "Optional suspension note")

	return cmd
}

func suspendDryRunParams(req suspendRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.SuspensionNote != "" {
		params.Set("suspension_note", req.SuspensionNote)
	}
	return params
}
