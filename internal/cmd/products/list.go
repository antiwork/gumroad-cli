package products

import (
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type productListItem struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Published      bool        `json:"published"`
	FormattedPrice string      `json:"formatted_price"`
	SalesCount     api.JSONInt `json:"sales_count"`
}

type productsListResponse struct {
	Products []productListItem `json:"products"`
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List products",
		Args:    cmdutil.ExactArgs(0),
		Example: `  gumroad products list`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[productsListResponse](opts, "Fetching products...", "GET", "/products", url.Values{}, func(resp productsListResponse) error {
				if len(resp.Products) == 0 {
					return cmdutil.PrintInfo(opts, "No products found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, p := range resp.Products {
						status := "draft"
						if p.Published {
							status = "published"
						}
						rows = append(rows, []string{p.ID, p.Name, status, p.FormattedPrice, fmt.Sprintf("%d", p.SalesCount)})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "NAME", "STATUS", "PRICE", "SALES")
				for _, p := range resp.Products {
					status := style.Yellow("draft")
					if p.Published {
						status = style.Green("published")
					}
					tbl.AddRow(p.ID, p.Name, status, p.FormattedPrice, fmt.Sprintf("%d", p.SalesCount))
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					if err := tbl.Render(w); err != nil {
						return err
					}
					if !opts.Quiet {
						return output.Writeln(w, style.Dim("\nTip: view a product with  gumroad products view <id>"))
					}
					return nil
				})
			})
		},
	}
}
