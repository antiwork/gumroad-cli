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
	ID                 string      `json:"id"`
	Name               string      `json:"name"`
	Published          bool        `json:"published"`
	FormattedPrice     string      `json:"formatted_price"`
	SalesCount         api.JSONInt `json:"sales_count"`
	IsTieredMembership bool        `json:"is_tiered_membership"`
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
				var memberships, standard []productListItem
				for _, p := range resp.Products {
					if p.IsTieredMembership {
						memberships = append(memberships, p)
					} else {
						standard = append(standard, p)
					}
				}

				buildTable := func(items []productListItem, countHeader string) *output.Table {
					tbl := output.NewStyledTable(style, "ID", "NAME", "STATUS", "PRICE", countHeader)
					for _, p := range items {
						status := style.Yellow("draft")
						if p.Published {
							status = style.Green("published")
						}
						tbl.AddRow(p.ID, p.Name, status, p.FormattedPrice, fmt.Sprintf("%d", p.SalesCount))
					}
					return tbl
				}

				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					split := len(memberships) > 0 && len(standard) > 0
					if split {
						if err := output.Writeln(w, style.Bold("Memberships")); err != nil {
							return err
						}
						if err := buildTable(memberships, "MEMBERS").Render(w); err != nil {
							return err
						}
						if err := output.Writeln(w, "\n"+style.Bold("Products")); err != nil {
							return err
						}
						if err := buildTable(standard, "SALES").Render(w); err != nil {
							return err
						}
					} else {
						items := standard
						header := "SALES"
						if len(memberships) > 0 {
							items = memberships
							header = "MEMBERS"
						}
						if err := buildTable(items, header).Render(w); err != nil {
							return err
						}
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
