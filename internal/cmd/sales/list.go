package sales

import (
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	tuisales "github.com/antiwork/gumroad-cli/internal/tui/sales"
	"github.com/spf13/cobra"
)

type saleListItem struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	ProductName    string `json:"product_name"`
	FormattedTotal string `json:"formatted_total_price"`
	CreatedAt      string `json:"created_at"`
	Refunded       bool   `json:"refunded"`
}

type salesListResponse struct {
	Success     bool           `json:"success"`
	Sales       []saleListItem `json:"sales"`
	NextPageKey string         `json:"next_page_key,omitempty"`
}

func newListCmd() *cobra.Command {
	var product, email, orderID, before, after, pageKey string
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sales",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad sales list
  gumroad sales list --product <id> --after 2024-01-01
  gumroad sales list --all
  gumroad sales list --json --jq '.sales[0].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireDateFlag(c, "before", before); err != nil {
				return err
			}
			if err := cmdutil.RequireDateFlag(c, "after", after); err != nil {
				return err
			}

			params := url.Values{}
			if product != "" {
				params.Set("product_id", product)
			}
			if email != "" {
				params.Set("email", email)
			}
			if orderID != "" {
				params.Set("order_id", orderID)
			}
			if before != "" {
				params.Set("before", before)
			}
			if after != "" {
				params.Set("after", after)
			}
			if pageKey != "" {
				params.Set("page_key", pageKey)
			}
			if all {
				return streamSalesListAll(opts, params)
			}

			return cmdutil.RunRequestDecoded[salesListResponse](opts, "Fetching sales...", "GET", "/sales", params, func(resp salesListResponse) error {
				return renderSalesList(opts, resp, product, email, orderID, before, after)
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Filter by product ID")
	cmd.Flags().StringVar(&email, "email", "", "Filter by buyer email")
	cmd.Flags().StringVar(&orderID, "order", "", "Filter by order ID")
	cmd.Flags().StringVar(&before, "before", "", "Filter sales before date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&after, "after", "", "Filter sales after date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&pageKey, "page-key", "", "Pagination cursor")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages")
	cmd.MarkFlagsMutuallyExclusive("all", "page-key")

	return cmd
}

func renderSalesList(opts cmdutil.Options, resp salesListResponse, product, email, orderID, before, after string) error {
	if len(resp.Sales) == 0 {
		return renderEmptySalesList(opts, product, email, orderID, before, after, resp.NextPageKey)
	}

	if opts.PlainOutput {
		return writeSalesPlain(opts.Out(), resp.Sales)
	}

	if opts.InteractiveTUIAllowed() {
		return runSalesTUI(opts, resp.Sales)
	}

	style := opts.Style()
	hint := salesPaginationHint(product, email, orderID, before, after, resp.NextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writeSalesTable(w, style, resp.Sales); err != nil {
			return err
		}
		if resp.NextPageKey != "" && !opts.Quiet {
			return output.Writeln(w, style.Dim("\nMore results available: "+hint))
		}
		return nil
	})
}

func runSalesTUI(opts cmdutil.Options, sales []saleListItem) error {
	model := make([]tuisales.Sale, 0, len(sales))
	for _, s := range sales {
		model = append(model, tuisales.Sale{
			ID:            s.ID,
			Email:         s.Email,
			Product:       s.ProductName,
			FormattedCost: s.FormattedTotal,
			CreatedAt:     s.CreatedAt,
			Refunded:      s.Refunded,
		})
	}
	return tuisales.Run(opts.In(), opts.Out(), model)
}

func streamSalesListAll(opts cmdutil.Options, params url.Values) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	sp := output.NewSpinnerTo("Fetching sales...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, token)
	style := opts.Style()
	walkPages := func(visit cmdutil.PageVisitor[salesListResponse]) error {
		return walkSalesPages(opts, client, params, visit)
	}

	return cmdutil.StreamPaginatedPages(opts, cmdutil.PaginatedPageOutputConfig[salesListResponse]{
		JSONKey:      "sales",
		EmptyMessage: "No sales found.",
		Walk:         walkPages,
		HasItems:     hasSales,
		WriteItems:   writeSalesItems,
		WritePlainPage: func(w io.Writer, page salesListResponse) error {
			return writeSalesPlain(w, page.Sales)
		},
		WriteTablePage: func(w io.Writer, page salesListResponse) error {
			return writeSalesTable(w, style, page.Sales)
		},
	})
}

func walkSalesPages(opts cmdutil.Options, client *api.Client, params url.Values, visit cmdutil.PageVisitor[salesListResponse]) error {
	return cmdutil.WalkPagesWithDelay[salesListResponse](opts.Context, opts.PageDelay, client, "/sales", params, func(page salesListResponse) string {
		return page.NextPageKey
	}, visit)
}

func hasSales(page salesListResponse) bool {
	return len(page.Sales) > 0
}

func writeSalesItems(page salesListResponse, writeItem func(any) error) error {
	for _, sale := range page.Sales {
		if err := writeItem(sale); err != nil {
			return err
		}
	}
	return nil
}

func writeSalesPlain(w io.Writer, sales []saleListItem) error {
	var rows [][]string
	for _, s := range sales {
		rows = append(rows, []string{s.ID, s.Email, s.ProductName, s.FormattedTotal, s.CreatedAt})
	}
	return output.PrintPlain(w, rows)
}

func writeSalesTable(w io.Writer, style output.Styler, sales []saleListItem) error {
	tbl := output.NewStyledTable(style, "ID", "EMAIL", "PRODUCT", "TOTAL", "DATE")
	for _, s := range sales {
		id := s.ID
		if s.Refunded {
			id = s.ID + " " + style.Red("(refunded)")
		}
		tbl.AddRow(id, s.Email, s.ProductName, s.FormattedTotal, s.CreatedAt)
	}
	return tbl.Render(w)
}

func renderEmptySalesList(opts cmdutil.Options, product, email, orderID, before, after, nextPageKey string) error {
	if nextPageKey == "" || opts.PlainOutput || opts.Quiet {
		return cmdutil.PrintInfo(opts, "No sales found.")
	}

	style := opts.Style()
	hint := salesPaginationHint(product, email, orderID, before, after, nextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, "No sales found on this page."); err != nil {
			return err
		}
		return output.Writeln(w, style.Dim("More results available: "+hint))
	})
}

func salesPaginationHint(product, email, orderID, before, after, nextPageKey string) string {
	return cmdutil.ReplayCommand("gumroad sales list",
		cmdutil.CommandArg{Flag: "--product", Value: product},
		cmdutil.CommandArg{Flag: "--email", Value: email},
		cmdutil.CommandArg{Flag: "--order", Value: orderID},
		cmdutil.CommandArg{Flag: "--before", Value: before},
		cmdutil.CommandArg{Flag: "--after", Value: after},
		cmdutil.CommandArg{Flag: "--page-key", Value: nextPageKey},
	)
}
