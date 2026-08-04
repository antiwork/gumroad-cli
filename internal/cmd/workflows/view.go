package workflows

import (
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type workflowViewResponse struct {
	Success  bool           `json:"success"`
	Workflow workflowRecord `json:"workflow"`
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View a workflow and its email steps",
		Long: "View a workflow and its email steps, with per-step delay, sent, open, and click stats.\n\n" +
			"Plain output prints one row per email step: id, subject, delay, state, sent, opens, open rate, clicks, click rate.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad workflows view <id>
  gumroad workflows view <id> --json
  gumroad workflows view <id> --plain`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[workflowViewResponse](opts, "Fetching workflow...", "GET", cmdutil.JoinPath("workflows", args[0]), url.Values{}, func(resp workflowViewResponse) error {
				return renderWorkflowView(opts, resp.Workflow)
			})
		},
	}
}

// Steps render in response order. The server orders them by
// delayed_delivery_time, a seconds value the API does not expose;
// a client-side sort on the displayed delay units could only
// approximate that order, so do not re-sort here.
func renderWorkflowView(opts cmdutil.Options, item workflowRecord) error {
	if opts.PlainOutput {
		var rows [][]string
		for _, email := range item.Emails {
			rows = append(rows, []string{
				email.ID,
				email.Subject,
				workflowDelayLabel(email.Delay),
				email.State,
				fmt.Sprintf("%d", email.SentCount),
				fmt.Sprintf("%d", email.OpenCount),
				workflowRateLabel(email.OpenRate),
				fmt.Sprintf("%d", email.ClickCount),
				workflowRateLabel(email.ClickRate),
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, style.Bold(item.Name)); err != nil {
			return err
		}
		lines := [][2]string{
			{"ID", item.ID},
			{"Audience", workflowAudienceLabel(item.AudienceType)},
			{"State", item.State},
		}
		if item.Trigger != "" {
			lines = append(lines, [2]string{"Trigger", item.Trigger})
		}
		if item.ProductID != "" {
			lines = append(lines, [2]string{"Product ID", item.ProductID})
		}
		if item.VariantID != "" {
			lines = append(lines, [2]string{"Variant ID", item.VariantID})
		}
		lines = append(lines, [2]string{"Send to past customers", workflowBool(item.SendToPastCustomers)})
		if item.PublishedAt != "" {
			lines = append(lines, [2]string{"Published at", item.PublishedAt})
		}
		for _, line := range lines {
			if err := output.Writef(w, "%s: %s\n", line[0], line[1]); err != nil {
				return err
			}
		}

		if len(item.Emails) == 0 {
			return output.Writeln(w, "No emails in this workflow.")
		}

		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		tbl := output.NewStyledTable(style, "ID", "SUBJECT", "DELAY", "STATE", "SENT", "OPENS", "OPEN %", "CLICKS", "CLICK %")
		for _, email := range item.Emails {
			tbl.AddRow(
				email.ID,
				email.Subject,
				workflowDelayLabel(email.Delay),
				email.State,
				fmt.Sprintf("%d", email.SentCount),
				fmt.Sprintf("%d", email.OpenCount),
				workflowRateLabel(email.OpenRate),
				fmt.Sprintf("%d", email.ClickCount),
				workflowRateLabel(email.ClickRate),
			)
		}
		return tbl.Render(w)
	})
}
