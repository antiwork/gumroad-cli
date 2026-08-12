package workflows

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

type workflowEmailResponse struct {
	Success bool                `json:"success"`
	Email   workflowEmailRecord `json:"email"`
}

func newAddEmailCmd() *cobra.Command {
	var subject, body, delay string

	cmd := &cobra.Command{
		Use:   "add-email <workflow-id>",
		Short: "Add an email step to a workflow",
		Long: `Add one email step to an existing workflow from an HTML body file.

The step is appended with the given delay; the workflow's publish state is not
changed, so adding a step never sends mail by itself. Publishing a workflow
stays a dashboard action. Abandoned cart workflows do not accept new steps.`,
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad workflows add-email <workflow-id> --subject "Week 4 check-in" --body ./email.html --delay "4 weeks"
  build-email | gumroad workflows add-email <workflow-id> --subject "From stdin" --body - --delay "1 hour"
  gumroad workflows add-email <workflow-id> --subject "Check params" --body ./email.html --delay "2 days" --dry-run`,
		RunE: func(c *cobra.Command, args []string) error {
			if subject == "" {
				return cmdutil.MissingFlagError(c, "--subject")
			}
			if body == "" {
				return cmdutil.MissingFlagError(c, "--body")
			}
			if delay == "" {
				return cmdutil.MissingFlagError(c, "--delay")
			}
			delayAmount, delayUnit, err := parseDelayFlag(delay)
			if err != nil {
				return cmdutil.UsageErrorf(c, "--delay: %v", err)
			}

			opts := cmdutil.OptionsFrom(c)
			input, err := pageutil.ReadHTML(opts.In(), body)
			if err != nil {
				return cmdutil.UsageErrorf(c, "--body: %v", err)
			}

			params := url.Values{}
			params.Set("subject", subject)
			params.Set("body", input.HTML)
			params.Set("delay_amount", strconv.Itoa(delayAmount))
			params.Set("delay_unit", delayUnit)

			path := cmdutil.JoinPath("workflows", args[0], "emails")
			return cmdutil.RunRequestDecoded[workflowEmailResponse](opts,
				"Adding email step...", "POST", path, params,
				func(resp workflowEmailResponse) error {
					return renderSavedEmail(opts, "Added email step:", resp.Email)
				})
		},
	}

	cmd.Flags().StringVar(&subject, "subject", "", "Email subject (required)")
	cmd.Flags().StringVar(&body, "body", "", "Path to an HTML body file, or - for stdin (required)")
	cmd.Flags().StringVar(&delay, "delay", "", `Delay after the trigger, as "<amount> <unit>" with unit hour, day, week, or month (required)`)

	return cmd
}

func renderSavedEmail(opts cmdutil.Options, label string, item workflowEmailRecord) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{item.ID, item.Subject, workflowDelayLabel(item.Delay), item.State}})
	}
	if opts.Quiet {
		return nil
	}
	style := opts.Style()
	return output.Writef(opts.Out(), "%s %s (%s) delay %s [%s]\n",
		style.Bold(label), item.Subject, style.Dim(item.ID), workflowDelayLabel(item.Delay), item.State)
}
