package payouts

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type resumeRequest struct {
	Email string `json:"email"`
}

func newResumeCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume payouts for a user as an admin",
		Long: `Resume internal payouts for a user. The server records a "Payouts resumed."
audit comment on the user automatically.`,
		Example: `  gumroad admin payouts resume --email seller@example.com
  gumroad admin payouts resume --email seller@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Resume payouts for "+email+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "resume payouts for "+email, email)
			}

			req := resumeRequest{Email: email}
			path := "payouts/resume"

			if opts.DryRun {
				params := url.Values{}
				params.Set("email", email)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Resuming payouts...", path, req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[payoutsActionResponse](data)
			if err != nil {
				return err
			}
			return renderPayoutsAction(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}
