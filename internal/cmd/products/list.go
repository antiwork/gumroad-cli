package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	salesCountHeader   = "SALES"
	membersCountHeader = "MEMBERS"
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
	RawProducts []json.RawMessage `json:"-"`
	NextPageKey string            `json:"next_page_key,omitempty"`
}

func (resp *productsListResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Products    []json.RawMessage `json:"products"`
		NextPageKey string            `json:"next_page_key,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	products := make([]productListItem, 0, len(raw.Products))
	for _, item := range raw.Products {
		var p productListItem
		if err := json.Unmarshal(item, &p); err != nil {
			return err
		}
		products = append(products, p)
	}

	resp.Products = products
	resp.RawProducts = raw.Products
	resp.NextPageKey = raw.NextPageKey
	return nil
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
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writeProductSections(w, style, resp.Products); err != nil {
			return err
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
			return writeProductSections(w, style, page.Products)
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
	if len(page.RawProducts) > 0 {
		for _, item := range page.RawProducts {
			if err := writeItem(item); err != nil {
				return err
			}
		}
		return nil
	}

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

func writeProductSections(w io.Writer, style output.Styler, products []productListItem) error {
	var memberships, standard []productListItem
	for _, p := range products {
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

	if len(memberships) > 0 && len(standard) > 0 {
		if err := output.Writeln(w, style.Bold("Memberships")); err != nil {
			return err
		}
		if err := buildTable(memberships, membersCountHeader).Render(w); err != nil {
			return err
		}
		if err := output.Writeln(w, "\n"+style.Bold("Products")); err != nil {
			return err
		}
		return buildTable(standard, salesCountHeader).Render(w)
	}

	items := standard
	header := salesCountHeader
	if len(memberships) > 0 {
		items = memberships
		header = membersCountHeader
	}
	return buildTable(items, header).Render(w)
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
