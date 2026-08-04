package workflows

import (
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type workflowListResponse struct {
	Success   bool             `json:"success"`
	Workflows []workflowRecord `json:"workflows"`
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List email workflows",
		Long:  "List email workflows with audience, state, and email step count.",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad workflows list
  gumroad workflows list --json --jq '.workflows[] | {id, name, emails_count}'
  gumroad workflows list --plain`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[workflowListResponse](opts, "Fetching workflows...", "GET", cmdutil.JoinPath("workflows"), url.Values{}, func(resp workflowListResponse) error {
				return renderWorkflowList(opts, resp)
			})
		},
	}
}

func renderWorkflowList(opts cmdutil.Options, resp workflowListResponse) error {
	if len(resp.Workflows) == 0 {
		return cmdutil.PrintInfo(opts, "No workflows found.")
	}

	if opts.PlainOutput {
		var rows [][]string
		for _, item := range resp.Workflows {
			rows = append(rows, []string{
				item.ID,
				item.Name,
				workflowAudienceLabel(item.AudienceType),
				item.State,
				fmt.Sprintf("%d", item.EmailsCount),
				item.ProductID,
				item.PublishedAt,
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		tbl := output.NewStyledTable(style, "ID", "NAME", "AUDIENCE", "STATE", "EMAILS", "PRODUCT", "PUBLISHED AT")
		for _, item := range resp.Workflows {
			tbl.AddRow(
				item.ID,
				item.Name,
				workflowAudienceLabel(item.AudienceType),
				item.State,
				fmt.Sprintf("%d", item.EmailsCount),
				item.ProductID,
				item.PublishedAt,
			)
		}
		return tbl.Render(w)
	})
}
