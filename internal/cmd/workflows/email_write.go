package workflows

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

const (
	maxWorkflowDelaySeconds int64 = (1 << 31) - 1
	secondsPerHour                = 60 * 60
	secondsPerDay                 = 24 * secondsPerHour
	secondsPerWeek                = 7 * secondsPerDay
	secondsPerMonth               = 2629746
)

var workflowDelaySeconds = map[string]int64{
	"hour":  secondsPerHour,
	"day":   secondsPerDay,
	"week":  secondsPerWeek,
	"month": secondsPerMonth,
}

type workflowEmailResponse struct {
	Success bool                `json:"success"`
	Email   workflowEmailRecord `json:"email"`
}

type workflowDelayInput struct {
	Amount int64
	Unit   string
}

func newAddEmailCmd() *cobra.Command {
	var subject, body, delay string

	cmd := &cobra.Command{
		Use:   "add-email <workflow-id>",
		Short: "Add an email step to a workflow",
		Long: `Add one email step from an HTML body file.

This command does not change the workflow publication state. A published
workflow can schedule eligible past recipients when you add the step. Pass
--yes to confirm the write.`,
		Example: `  gumroad workflows add-email <workflow-id> --subject "Week four" --body ./email.html --delay "4 weeks" --yes
  build-email | gumroad workflows add-email <workflow-id> --subject "Welcome" --body - --delay "0 hours" --yes
  gumroad workflows add-email <workflow-id> --subject "Check params" --body ./email.html --delay "1 day" --dry-run`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if strings.TrimSpace(subject) == "" {
				return cmdutil.MissingFlagError(c, "--subject")
			}
			if body == "" {
				return cmdutil.MissingFlagError(c, "--body")
			}
			if delay == "" {
				return cmdutil.MissingFlagError(c, "--delay")
			}

			opts := cmdutil.OptionsFrom(c)
			if err := output.ValidateJQExpression(opts.JQExpr); err != nil {
				return err
			}

			parsedDelay, err := parseWorkflowDelay(delay)
			if err != nil {
				return cmdutil.UsageErrorf(c, "--delay: %v", err)
			}

			input, err := pageutil.ReadHTML(opts.In(), body)
			if err != nil {
				return cmdutil.UsageErrorf(c, "--body: %v", err)
			}

			params := url.Values{}
			params.Set("subject", subject)
			params.Set("body", input.HTML)
			setWorkflowDelayParams(params, parsedDelay)

			ok, err := cmdutil.ConfirmAction(opts, "Add an email step to workflow "+args[0]+"? A published workflow can schedule eligible past recipients.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "add workflow email", "")
			}

			return runWorkflowEmailWrite(
				opts,
				"Adding workflow email...",
				http.MethodPost,
				cmdutil.JoinPath("workflows", args[0], "emails"),
				params,
				EmailWriteActionAdd,
				args[0],
				"",
				"Added workflow email:",
			)
		},
	}

	cmd.Flags().StringVar(&subject, "subject", "", "Email subject (required)")
	cmd.Flags().StringVar(&body, "body", "", "Path to an HTML body file, or - for stdin (required)")
	cmd.Flags().StringVar(&delay, "delay", "", `Delay such as "4 weeks" (required)`)

	return cmd
}

func newUpdateEmailCmd() *cobra.Command {
	var subject, body, delay string

	cmd := &cobra.Command{
		Use:   "update-email <workflow-id> <email-id>",
		Short: "Update an email step in a workflow",
		Long: `Update selected fields on one workflow email step.

Omitted fields stay unchanged. This command does not change the workflow
publication state. A delay change can reschedule recipients and requires
confirmation.`,
		Example: `  gumroad workflows update-email <workflow-id> <email-id> --body ./email.html
  gumroad workflows update-email <workflow-id> <email-id> --subject "New subject" --delay "2 days" --yes
  build-email | gumroad workflows update-email <workflow-id> <email-id> --body -
  gumroad workflows update-email <workflow-id> <email-id> --delay "1 week" --dry-run`,
		Args: cmdutil.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			if err := cmdutil.RequireAnyFlagChanged(c, "subject", "body", "delay"); err != nil {
				return err
			}
			if c.Flags().Changed("subject") && strings.TrimSpace(subject) == "" {
				return cmdutil.UsageErrorf(c, "--subject must be a non-empty string")
			}
			if c.Flags().Changed("body") && body == "" {
				return cmdutil.UsageErrorf(c, "--body must be a file path or - for stdin")
			}

			opts := cmdutil.OptionsFrom(c)
			if err := output.ValidateJQExpression(opts.JQExpr); err != nil {
				return err
			}

			params := url.Values{}
			if c.Flags().Changed("subject") {
				params.Set("subject", subject)
			}
			if c.Flags().Changed("delay") {
				parsedDelay, err := parseWorkflowDelay(delay)
				if err != nil {
					return cmdutil.UsageErrorf(c, "--delay: %v", err)
				}
				setWorkflowDelayParams(params, parsedDelay)
			}

			if c.Flags().Changed("body") {
				input, err := pageutil.ReadHTML(opts.In(), body)
				if err != nil {
					return cmdutil.UsageErrorf(c, "--body: %v", err)
				}
				params.Set("body", input.HTML)
			}
			if c.Flags().Changed("delay") {
				ok, err := cmdutil.ConfirmAction(opts, "Change the delay for email "+args[1]+"? The change can reschedule recipients.")
				if err != nil {
					return err
				}
				if !ok {
					return cmdutil.PrintCancelledAction(opts, "change workflow email delay", args[1])
				}
			}

			return runWorkflowEmailWrite(
				opts,
				"Updating workflow email...",
				http.MethodPut,
				cmdutil.JoinPath("workflows", args[0], "emails", args[1]),
				params,
				EmailWriteActionUpdate,
				args[0],
				args[1],
				"Updated workflow email:",
			)
		},
	}

	cmd.Flags().StringVar(&subject, "subject", "", "New email subject")
	cmd.Flags().StringVar(&body, "body", "", "Path to a new HTML body file, or - for stdin")
	cmd.Flags().StringVar(&delay, "delay", "", `New delay such as "4 weeks"`)

	return cmd
}

func parseWorkflowDelay(value string) (workflowDelayInput, error) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return workflowDelayInput{}, fmt.Errorf(`must use "<non-negative integer> <hour|day|week|month>"`)
	}

	amount, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || amount < 0 {
		return workflowDelayInput{}, fmt.Errorf("amount must be a non-negative integer")
	}

	unit := strings.ToLower(parts[1])
	unit = strings.TrimSuffix(unit, "s")
	seconds, ok := workflowDelaySeconds[unit]
	if !ok {
		return workflowDelayInput{}, fmt.Errorf("unit must be one of: hour, day, week, month")
	}
	if amount > maxWorkflowDelaySeconds/seconds {
		return workflowDelayInput{}, fmt.Errorf("total must not exceed %d seconds", maxWorkflowDelaySeconds)
	}

	return workflowDelayInput{Amount: amount, Unit: unit}, nil
}

func setWorkflowDelayParams(params url.Values, delay workflowDelayInput) {
	params.Set("delay_amount", strconv.FormatInt(delay.Amount, 10))
	params.Set("delay_unit", delay.Unit)
}

func runWorkflowEmailWrite(
	opts cmdutil.Options,
	spinnerMessage, method, path string,
	params url.Values,
	action EmailWriteAction,
	workflowID, emailID, label string,
) error {
	if opts.DryRun {
		return cmdutil.PrintDryRunRequest(opts, method, path, params)
	}

	token, err := config.Token()
	if err != nil {
		return err
	}
	data, err := cmdutil.RunRequestWithTokenData(opts, token, spinnerMessage, method, path, params)
	if err != nil {
		return emailWriteRequestError(err, action, workflowID, emailID)
	}
	resp, err := cmdutil.DecodeJSON[workflowEmailResponse](data)
	if err != nil || resp.Email.ID == "" {
		if err == nil {
			err = fmt.Errorf("workflow email response did not include email.id")
		}
		return newUnknownEmailWriteError(err, action, workflowID, emailID)
	}

	if err := renderWorkflowEmailWrite(opts, data, resp, label); err != nil {
		return emailWriteOutputError(err, action, workflowID, resp.Email.ID)
	}
	return nil
}

func renderWorkflowEmailWrite(opts cmdutil.Options, data []byte, resp workflowEmailResponse, label string) error {
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	item := resp.Email
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{item.ID, item.Subject, workflowDelayLabel(item.Delay), item.State}})
	}
	if opts.Quiet {
		return nil
	}
	style := opts.Style()
	return output.Writef(opts.Out(), "%s %s (%s) [%s]\n", style.Bold(label), item.Subject, style.Dim(item.ID), item.State)
}
