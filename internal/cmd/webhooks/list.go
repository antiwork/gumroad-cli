package webhooks

import (
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type webhookListItem struct {
	ID           string `json:"id"`
	ResourceName string `json:"resource_name"`
	PostURL      string `json:"post_url"`
}

type webhooksListResponse struct {
	ResourceSubscriptions []webhookListItem `json:"resource_subscriptions"`
}

func newListCmd() *cobra.Command {
	var resource string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhook subscriptions",
		Args:  cmdutil.ExactArgs(0),
		Long:  "List webhook subscriptions. --resource is required (no 'list all' endpoint).",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if resource == "" {
				return cmdutil.MissingFlagError(c, "--resource")
			}

			params := url.Values{}
			params.Set("resource_name", resource)

			return cmdutil.RunRequestDecoded[webhooksListResponse](opts, "Fetching webhooks...", "GET", "/resource_subscriptions", params, func(resp webhooksListResponse) error {
				if len(resp.ResourceSubscriptions) == 0 {
					return cmdutil.PrintInfo(opts, "No webhooks found for resource: "+resource)
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, rs := range resp.ResourceSubscriptions {
						rows = append(rows, []string{rs.ID, rs.ResourceName, rs.PostURL})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "RESOURCE", "URL")
				for _, rs := range resp.ResourceSubscriptions {
					tbl.AddRow(rs.ID, rs.ResourceName, rs.PostURL)
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}

	cmd.Flags().StringVar(&resource, "resource", "", "Resource name (required)")

	return cmd
}
