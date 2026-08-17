package emails

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUnscheduleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unschedule <id>",
		Short: "Unschedule an audience email",
		Long:  "Cancel the scheduled send time on an audience email, returning it to draft so it can be edited or re-scheduled.",
		Example: `  gumroad emails unschedule <id>
  gumroad emails unschedule <id> --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestWithResource(opts, "Unscheduling email...", "POST", cmdutil.JoinPath("emails", args[0], "unschedule"), url.Values{}, "", "Email "+args[0]+" unscheduled.")
		},
	}
}
