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

type scheduledExecuteResponse struct {
	Success         bool            `json:"success"`
	Result          string          `json:"result"`
	Message         string          `json:"message"`
	ScheduledPayout scheduledPayout `json:"scheduled_payout"`
}

func newScheduledExecuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute <external_id>",
		Short: "Execute a pending or flagged scheduled payout",
		Long: `Execute a ScheduledPayout by its external id. Only pending or flagged rows
can be executed. The server may return a result of executed, held, or flagged.

This moves money. --yes is required.`,
		Example: `  gumroad admin payouts scheduled execute pay_abc123 --yes
  gumroad admin payouts scheduled execute pay_abc123 --yes --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			id := args[0]

			path := cmdutil.JoinPath("payouts", id, "scheduled_execute")

			ok, err := cmdutil.ConfirmAction(opts, "Execute scheduled payout "+id+"? This moves money.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "execute scheduled payout "+id, id)
			}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), url.Values{})
			}

			data, err := admincmd.FetchPostJSON(opts, "Executing scheduled payout...", path, struct{}{})
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[scheduledExecuteResponse](data)
			if err != nil {
				return err
			}
			return renderScheduledExecute(opts, id, decoded)
		},
	}

	return cmd
}

func renderScheduledExecute(opts cmdutil.Options, id string, resp scheduledExecuteResponse) error {
	headline := fallbackStr(resp.Message, "Executed scheduled payout "+id)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			"true", headline, id, resp.Result, resp.ScheduledPayout.Status,
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(headline)); err != nil {
		return err
	}
	if resp.Result != "" {
		if err := output.Writef(opts.Out(), "Result: %s\n", resp.Result); err != nil {
			return err
		}
	}
	if resp.ScheduledPayout.Status != "" {
		return output.Writef(opts.Out(), "Status: %s\n", resp.ScheduledPayout.Status)
	}
	return nil
}
