package products

import (
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
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
	Products    []productListItem `json:"products"`
	NextPageKey string            `json:"next_page_key,omitempty"`
}

func newListCmd() *cobra.Command {
	var pageKey string
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List products",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad products list
  gumroad products list --all
  gumroad products list --page-key <cursor>
  gumroad products list --json --jq '.products[0].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			params := url.Values{}
			if pageKey != "" {
				params.Set("page_key", pageKey)
			}
			if all {
				return streamProductsListAll(opts, params)
			}

			return cmdutil.RunRequestDecoded[productsListResponse](opts, "Fetching products...", "GET", "/products", params, func(resp productsListResponse) error {
				return renderProductsList(opts, resp)
			})
		},
	}

	cmd.Flags().StringVar(&pageKey, "page-key", "", "Pagination cursor")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages")
	cmd.MarkFlagsMutuallyExclusive("all", "page-key")

	return cmd
}

func renderProductsList(opts cmdutil.Options, resp productsListResponse) error {
	if len(resp.Products) == 0 {
		if resp.NextPageKey != "" && !opts.PlainOutput && !opts.Quiet {
			style := opts.Style()
			hint := productsPaginationHint(resp.NextPageKey)
			return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
				if err := output.Writeln(w, "No products found on this page."); err != nil {
					return err
				}
				return output.Writeln(w, style.Dim("More results available: "+hint))
			})
		}
		return cmdutil.PrintInfo(opts, "No products found.")
	}

	if opts.PlainOutput {
		return writeProductsPlain(opts.Out(), resp.Products)
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
		addProductRows(tbl, style, items)
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
		if resp.NextPageKey != "" && !opts.Quiet {
			hint := productsPaginationHint(resp.NextPageKey)
			return output.Writeln(w, style.Dim("\nMore results available: "+hint))
		}
		if !opts.Quiet {
			return output.Writeln(w, style.Dim("\nTip: view a product with  gumroad products view <id>"))
		}
		return nil
	})
}

func streamProductsListAll(opts cmdutil.Options, params url.Values) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	sp := output.NewSpinnerTo("Fetching products...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, token)
	style := opts.Style()
	walkPages := func(visit cmdutil.PageVisitor[productsListResponse]) error {
		return walkProductsPages(opts, client, params, visit)
	}

	return cmdutil.StreamPaginatedPages(opts, cmdutil.PaginatedPageOutputConfig[productsListResponse]{
		JSONKey:      "products",
		EmptyMessage: "No products found.",
		Walk:         walkPages,
		HasItems:     hasProducts,
		WriteItems:   writeProductItems,
		WritePlainPage: func(w io.Writer, page productsListResponse) error {
			return writeProductsPlain(w, page.Products)
		},
		WriteTablePage: func(w io.Writer, page productsListResponse) error {
			return writeProductsTable(w, style, page.Products)
		},
	})
}

func walkProductsPages(opts cmdutil.Options, client *api.Client, params url.Values, visit cmdutil.PageVisitor[productsListResponse]) error {
	return cmdutil.WalkPagesWithDelay[productsListResponse](opts.Context, opts.PageDelay, client, "/products", params, func(page productsListResponse) string {
		return page.NextPageKey
	}, visit)
}

func hasProducts(page productsListResponse) bool {
	return len(page.Products) > 0
}

func writeProductItems(page productsListResponse, writeItem func(any) error) error {
	for _, p := range page.Products {
		if err := writeItem(p); err != nil {
			return err
		}
	}
	return nil
}

func writeProductsPlain(w io.Writer, products []productListItem) error {
	var rows [][]string
	for _, p := range products {
		status := "draft"
		if p.Published {
			status = "published"
		}
		rows = append(rows, []string{p.ID, p.Name, status, p.FormattedPrice, fmt.Sprintf("%d", p.SalesCount)})
	}
	return output.PrintPlain(w, rows)
}

func writeProductsTable(w io.Writer, style output.Styler, products []productListItem) error {
	tbl := output.NewStyledTable(style, "ID", "NAME", "STATUS", "PRICE", "SALES")
	addProductRows(tbl, style, products)
	return tbl.Render(w)
}

func addProductRows(tbl *output.Table, style output.Styler, products []productListItem) {
	for _, p := range products {
		status := style.Yellow("draft")
		if p.Published {
			status = style.Green("published")
		}
		tbl.AddRow(p.ID, p.Name, status, p.FormattedPrice, fmt.Sprintf("%d", p.SalesCount))
	}
}

func productsPaginationHint(nextPageKey string) string {
	return cmdutil.ReplayCommand("gumroad products list",
		cmdutil.CommandArg{Flag: "--page-key", Value: nextPageKey},
	)
}
