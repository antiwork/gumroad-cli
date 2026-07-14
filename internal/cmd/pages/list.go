package pages

import (
	"io"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type pagesListResponse struct {
	Success bool   `json:"success"`
	Pages   []page `json:"pages"`
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your storefront pages",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad pages list
  gumroad pages list --json --jq '.pages[].slug'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[pagesListResponse](opts, "Fetching pages...", http.MethodGet, pagesPath, url.Values{}, func(resp pagesListResponse) error {
				if len(resp.Pages) == 0 {
					return cmdutil.PrintInfo(opts, "No pages yet. Create one with `gumroad pages create --title <title>`.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, p := range resp.Pages {
						rows = append(rows, []string{p.Slug, p.Title, pageKind(p), pageURL(p)})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "SLUG", "TITLE", "KIND", "URL")
				for _, p := range resp.Pages {
					tbl.AddRow(p.Slug, p.Title, pageKind(p), pageURL(p))
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}
}
