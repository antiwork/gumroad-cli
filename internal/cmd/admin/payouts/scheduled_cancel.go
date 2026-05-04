package payouts

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type scheduledCancelResponse struct {
	Success         bool            `json:"success"`
	Message         string          `json:"message"`
	ScheduledPayout scheduledPayout `json:"scheduled_payout"`
}

func newScheduledCancelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <external_id>",
		Short: "Cancel a scheduled payout",
		Example: `  gumroad admin payouts scheduled cancel pay_abc123
  gumroad admin payouts scheduled cancel pay_abc123 --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			id := args[0]

			path := cmdutil.JoinPath("payouts", id, "scheduled_cancel")

			ok, err := cmdutil.ConfirmAction(opts, "Cancel scheduled payout "+id+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "cancel scheduled payout "+id, id)
			}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), url.Values{})
			}

			data, err := admincmd.FetchPostJSON(opts, "Cancelling scheduled payout...", path, struct{}{})
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[scheduledCancelResponse](data)
			if err != nil {
				return err
			}
			return renderScheduledCancel(opts, id, decoded)
		},
	}

	return cmd
}

func renderScheduledCancel(opts cmdutil.Options, id string, resp scheduledCancelResponse) error {
	headline := fallbackStr(resp.Message, "Cancelled scheduled payout "+id)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			"true", headline, id, resp.ScheduledPayout.Status,
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(headline)); err != nil {
		return err
	}
	if resp.ScheduledPayout.Status != "" {
		return output.Writef(opts.Out(), "Status: %s\n", resp.ScheduledPayout.Status)
	}
	return nil
}
