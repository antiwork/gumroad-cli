package emails

import (
	"net/url"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type scheduleEmailResponse struct {
	Email emailRecord `json:"email"`
}

func newScheduleCmd() *cobra.Command {
	var at string

	cmd := &cobra.Command{
		Use:   "schedule <id> --at <timestamp>",
		Short: "Schedule an audience email",
		Long: `Schedule an audience email to send at an absolute time.

The timestamp is passed through to the API verbatim and must be a parseable
form (e.g. RFC3339 like 2026-06-18T14:00:00Z). Calling schedule again with a
different --at reschedules the email.`,
		Example: `  gumroad emails schedule <id> --at "2026-06-18T14:00:00Z"
  gumroad emails schedule <id> --at "2026-06-18 14:00"
  gumroad emails schedule <id> --at "2026-06-18T14:00:00Z" --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if at == "" {
				return cmdutil.MissingFlagError(c, "--at")
			}
			if _, err := time.Parse(time.RFC3339, at); err != nil {
				return cmdutil.UsageErrorf(c, "--at: %v", err)
			}

			opts := cmdutil.OptionsFrom(c)
			params := url.Values{}
			params.Set("to_be_published_at", at)

			return cmdutil.RunRequestDecoded[scheduleEmailResponse](opts, "Scheduling email...", "POST", cmdutil.JoinPath("emails", args[0], "schedule"), params, func(resp scheduleEmailResponse) error {
				item := resp.Email
				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{{item.ID, item.Subject, item.State, item.ScheduledAt}})
				}
				if opts.Quiet {
					return nil
				}
				style := opts.Style()
				return output.Writef(opts.Out(), "%s %s (%s) [%s]\n",
					style.Bold("Scheduled email:"), item.Subject, style.Dim(item.ID), item.State)
			})
		},
	}

	cmd.Flags().StringVar(&at, "at", "", "When to send the email (required; RFC3339 timestamp)")

	return cmd
}
