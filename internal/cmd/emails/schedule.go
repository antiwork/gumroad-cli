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
		Long: `Schedule an audience email to send at a future time.

The timestamp may be RFC3339 (e.g. 2026-06-18T14:00:00Z) or a naive
seller-timezone time (e.g. 2026-06-18 14:00); the API resolves the latter in
the seller's timezone. Calling schedule again with a different timestamp
reschedules the email.`,
		Example: `  gumroad emails schedule <id> --at "2026-06-18T14:00:00Z"
  gumroad emails schedule <id> --to-be-published-at "2026-06-18 14:00"
  gumroad emails schedule <id> --at "2026-06-18T14:00:00Z" --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if at == "" {
				return cmdutil.MissingFlagError(c, "--at")
			}
			if !isValidScheduleTime(at) {
				return cmdutil.UsageErrorf(c, "--at %q is not a valid timestamp (e.g. 2026-06-18T14:00:00Z or 2026-06-18 14:00)", at)
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
				timeText := emailDisplayDate(item)
				if timeText != "" {
					return output.Writef(opts.Out(), "%s %s (%s) [%s] at %s\n",
						style.Bold("Scheduled email:"), item.Subject, style.Dim(item.ID), item.State, timeText)
				}
				return output.Writef(opts.Out(), "%s %s (%s) [%s]\n",
					style.Bold("Scheduled email:"), item.Subject, style.Dim(item.ID), item.State)
			})
		},
	}

	cmd.Flags().StringVar(&at, "at", "", "When to send the email (required; RFC3339 or seller-timezone timestamp)")
	cmd.Flags().StringVar(&at, "to-be-published-at", "", "Alias for --at")

	return cmd
}

// isValidScheduleTime reports whether value parses as a timestamp, accepting
// both RFC3339 and the naive seller-timezone form the API documents.
func isValidScheduleTime(value string) bool {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}
