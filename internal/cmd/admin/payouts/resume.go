package payouts

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type resumeRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

func newResumeCmd() *cobra.Command {
	var targetFlags mutationFlags

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume payouts for a user as an admin",
		Long: `Resume internal payouts for a user. The server records a "Payouts resumed."
audit comment on the user automatically.`,
		Example: `  gumroad admin payouts resume --user-id 2245593582708
  gumroad admin payouts resume --user-id 2245593582708 --expected-email seller@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			ok, err := cmdutil.ConfirmAction(opts, "Resume payouts for user_id "+target.UserID+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "resume payouts for user_id "+target.UserID, target.UserID)
			}

			req := resumeRequest(target)
			path := "payouts/resume"

			if opts.DryRun {
				params := mutationParams(target)
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
			return renderPayoutsAction(opts, fallbackStr(decoded.UserID, target.UserID), decoded)
		},
	}

	addMutationFlags(cmd, &targetFlags)

	return cmd
}
