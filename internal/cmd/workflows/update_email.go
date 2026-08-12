package workflows

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newUpdateEmailCmd() *cobra.Command {
	var subject, body, delay string

	cmd := &cobra.Command{
		Use:   "update-email <workflow-id> <email-id>",
		Short: "Update an email step in a workflow",
		Long: `Update one existing email step's subject, body, or delay in place.

Only the flags you pass change; other fields keep their current values. The
workflow's publish state is not touched. Delay changes are rejected for
abandoned cart workflows.`,
		Args: cmdutil.ExactArgs(2),
		Example: `  gumroad workflows update-email <workflow-id> <email-id> --body ./email.html
  gumroad workflows update-email <workflow-id> <email-id> --subject "New subject" --delay "2 weeks"
  gumroad workflows update-email <workflow-id> <email-id> --body ./email.html --dry-run`,
		RunE: func(c *cobra.Command, args []string) error {
			if err := cmdutil.RequireAnyFlagChanged(c, "subject", "body", "delay"); err != nil {
				return err
			}
			if c.Flags().Changed("subject") && subject == "" {
				return cmdutil.UsageErrorf(c, "--subject cannot be empty")
			}

			params := url.Values{}
			if c.Flags().Changed("subject") {
				params.Set("subject", subject)
			}
			if c.Flags().Changed("delay") {
				delayAmount, delayUnit, err := parseDelayFlag(delay)
				if err != nil {
					return cmdutil.UsageErrorf(c, "--delay: %v", err)
				}
				params.Set("delay_amount", strconv.Itoa(delayAmount))
				params.Set("delay_unit", delayUnit)
			}

			opts := cmdutil.OptionsFrom(c)
			if c.Flags().Changed("body") {
				input, err := pageutil.ReadHTML(opts.In(), body)
				if err != nil {
					return cmdutil.UsageErrorf(c, "--body: %v", err)
				}
				params.Set("body", input.HTML)
			}

			path := cmdutil.JoinPath("workflows", args[0], "emails", args[1])
			return cmdutil.RunRequestDecoded[workflowEmailResponse](opts,
				"Updating email step...", "PUT", path, params,
				func(resp workflowEmailResponse) error {
					return renderSavedEmail(opts, "Updated email step:", resp.Email)
				})
		},
	}

	cmd.Flags().StringVar(&subject, "subject", "", "New email subject")
	cmd.Flags().StringVar(&body, "body", "", "Path to an HTML body file, or - for stdin")
	cmd.Flags().StringVar(&delay, "delay", "", `New delay after the trigger, as "<amount> <unit>" with unit hour, day, week, or month`)

	return cmd
}
