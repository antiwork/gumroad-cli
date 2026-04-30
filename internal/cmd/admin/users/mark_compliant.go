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
	Email string `json:"email"`
	Note  string `json:"note,omitempty"`
}

func newMarkCompliantCmd() *cobra.Command {
	var (
		email string
		note  string
	)

	cmd := &cobra.Command{
		Use:   "mark-compliant",
		Short: "Mark a user compliant as an admin",
		Long:  "Mark a user compliant through the internal admin API.",
		Example: `  gumroad admin users mark-compliant --email seller@example.com
  gumroad admin users mark-compliant --email seller@example.com --note "Cleared after review"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Mark user "+email+" compliant?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "mark user "+email+" compliant", email)
			}

			req := markCompliantRequest{
				Email: email,
				Note:  note,
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
			return renderRiskAction(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&note, "note", "", "Optional admin note")

	return cmd
}

func markCompliantDryRunParams(req markCompliantRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.Note != "" {
		params.Set("note", req.Note)
	}
	return params
}
