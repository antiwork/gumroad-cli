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
	Email      string `json:"email,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Note       string `json:"note,omitempty"`
}

func newMarkCompliantCmd() *cobra.Command {
	var (
		email      string
		externalID string
		note       string
	)

	cmd := &cobra.Command{
		Use:   "mark-compliant",
		Short: "Mark a user compliant as an admin",
		Long: `Mark a user compliant through the internal admin API.

Identify the user with --email or --external-id. When both are supplied, the
server resolves by --external-id.`,
		Example: `  gumroad admin users mark-compliant --email seller@example.com
  gumroad admin users mark-compliant --external-id 2245593582708
  gumroad admin users mark-compliant --email seller@example.com --note "Cleared after review"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			if err := requireEmailOrExternalID(c, email, externalID); err != nil {
				return err
			}

			identifier := userIdentifier(email, externalID)
			ok, err := cmdutil.ConfirmAction(opts, "Mark user "+identifier+" compliant?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "mark user "+identifier+" compliant", identifier)
			}

			req := markCompliantRequest{
				Email:      email,
				ExternalID: externalID,
				Note:       note,
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
			return renderRiskAction(opts, riskActionLabel(email, externalID), identifier, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "User external ID")
	cmd.Flags().StringVar(&note, "note", "", "Optional admin note")

	return cmd
}

func markCompliantDryRunParams(req markCompliantRequest) url.Values {
	params := url.Values{}
	if req.Email != "" {
		params.Set("email", req.Email)
	}
	if req.ExternalID != "" {
		params.Set("external_id", req.ExternalID)
	}
	if req.Note != "" {
		params.Set("note", req.Note)
	}
	return params
}
