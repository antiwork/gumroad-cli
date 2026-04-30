package payouts

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type pauseRequest struct {
	Email  string `json:"email"`
	Reason string `json:"reason,omitempty"`
}

func newPauseCmd() *cobra.Command {
	var (
		email  string
		reason string
	)

	cmd := &cobra.Command{
		Use:   "pause",
		Short: "Pause payouts for a user as an admin",
		Long: `Pause internal payouts for a user. Pass --reason to record an audit comment
on the user explaining why payouts were paused; the comment is omitted when no
reason is provided.`,
		Example: `  gumroad admin payouts pause --email seller@example.com
  gumroad admin payouts pause --email seller@example.com --reason "Verification pending"
  gumroad admin payouts pause --email seller@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			confirmMsg := "Pause payouts for " + email + "?"
			if reason != "" {
				confirmMsg = "Pause payouts for " + email + "? (reason will be recorded)"
			}
			ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "pause payouts for "+email, email)
			}

			req := pauseRequest{Email: email, Reason: reason}
			path := "payouts/pause"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), pauseDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Pausing payouts...", path, req)
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
	cmd.Flags().StringVar(&reason, "reason", "", "Audit comment recorded against the user")

	return cmd
}

func pauseDryRunParams(req pauseRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.Reason != "" {
		params.Set("reason", req.Reason)
	}
	return params
}
