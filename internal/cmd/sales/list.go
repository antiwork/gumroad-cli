package sales

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type buyerPresentment struct {
	Currency      string       `json:"currency"`
	TotalCents    *nullableInt `json:"total_cents"`
	RefundedCents *nullableInt `json:"refunded_cents"`
	FxRate        string       `json:"fx_rate"`

	// raw keeps the exact API object so JSON output round-trips every field,
	// not just the ones this struct names (the API also sends price_cents,
	// tip_cents, tax and shipping amounts).
	raw json.RawMessage
}

func (p *buyerPresentment) UnmarshalJSON(data []byte) error {
	type plain buyerPresentment
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = buyerPresentment(decoded)
	p.raw = append(json.RawMessage(nil), data...)
	return nil
}

func (p buyerPresentment) MarshalJSON() ([]byte, error) {
	if len(p.raw) > 0 {
		return p.raw, nil
	}
	type plain buyerPresentment
	return json.Marshal(plain(p))
}

func (p *buyerPresentment) formattedTotal() string {
	if p == nil || p.TotalCents == nil || !p.TotalCents.valid {
		return ""
	}
	return fmt.Sprintf("%s %s", cmdutil.FormatMoney(int(p.TotalCents.value), p.Currency), strings.ToUpper(p.Currency))
}

type saleListItem struct {
	ID                  string            `json:"id"`
	Email               string            `json:"email"`
	ProductName         string            `json:"product_name"`
	FormattedTotal      string            `json:"formatted_total_price"`
	CreatedAt           string            `json:"created_at"`
	Refunded            bool              `json:"refunded"`
	TotalCents          *nullableInt      `json:"total_cents"`
	Price               *nullableInt      `json:"price"`
	Currency            string            `json:"currency"`
	CurrencyType        string            `json:"currency_type"`
	PriceCurrencyType   string            `json:"price_currency_type"`
	RefundedCents       *nullableInt      `json:"refunded_cents"`
	AmountRefundedCents *nullableInt      `json:"amount_refunded_cents"`
	BuyerPresentment    *buyerPresentment `json:"buyer_presentment,omitempty"`
}

type salesListResponse struct {
	Success     bool           `json:"success"`
	Sales       []saleListItem `json:"sales"`
	NextPageKey string         `json:"next_page_key,omitempty"`
}

type nullableInt struct {
	value api.JSONInt
	valid bool
}

func (n *nullableInt) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.valid = false
		n.value = 0
		return nil
	}

	if err := n.value.UnmarshalJSON(data); err != nil {
		return err
	}
	n.valid = true
	return nil
}

func (n nullableInt) MarshalJSON() ([]byte, error) {
	if !n.valid {
		return []byte("null"), nil
	}
	return []byte(strconv.Itoa(int(n.value))), nil
}

func newListCmd() *cobra.Command {
	var product, email, orderID, before, after, pageKey string
	var all, csvOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sales",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad sales list
  gumroad sales list --product <id> --after 2024-01-01
  gumroad sales list --after 2024-01-01 --csv
  gumroad sales list --all
  gumroad sales list --json --jq '.sales[0].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := validateSalesCSVOutput(c, opts, csvOutput); err != nil {
				return err
			}
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
				return streamSalesListAll(opts, params, csvOutput)
			}

			return cmdutil.RunRequestDecoded[salesListResponse](opts, "Fetching sales...", "GET", "/sales", params, func(resp salesListResponse) error {
				return renderSalesList(opts, resp, product, email, orderID, before, after, csvOutput)
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
	cmd.Flags().BoolVar(&csvOutput, "csv", false, "Output as CSV")
	cmd.MarkFlagsMutuallyExclusive("all", "page-key")

	return cmd
}

func validateSalesCSVOutput(cmd *cobra.Command, opts cmdutil.Options, csvOutput bool) error {
	if !csvOutput {
		return nil
	}
	if opts.JSONOutput || opts.JQExpr != "" || opts.PlainOutput {
		return cmdutil.NewUsageError(cmd, "--csv cannot be combined with --json, --jq, or --plain")
	}
	return nil
}

func renderSalesList(opts cmdutil.Options, resp salesListResponse, product, email, orderID, before, after string, csvOutput bool) error {
	if csvOutput {
		if err := writeSalesCSV(opts.Out(), resp.Sales); err != nil {
			return err
		}
		return renderSalesCSVPageHint(opts, product, email, orderID, before, after, resp.NextPageKey)
	}

	if len(resp.Sales) == 0 {
		return renderEmptySalesList(opts, product, email, orderID, before, after, resp.NextPageKey)
	}

	if opts.PlainOutput {
		return writeSalesPlain(opts.Out(), resp.Sales)
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

func renderSalesCSVPageHint(opts cmdutil.Options, product, email, orderID, before, after, nextPageKey string) error {
	if nextPageKey == "" || opts.Quiet {
		return nil
	}

	hint := salesCSVAllHint(product, email, orderID, before, after)
	return output.Writeln(opts.Err(), opts.Style().Dim("More results available: "+hint))
}

func streamSalesListAll(opts cmdutil.Options, params url.Values, csvOutput bool) error {
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

	if csvOutput {
		return streamSalesCSV(opts.Out(), walkPages)
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

var salesCSVHeader = []string{"id", "email", "product_name", "total_cents", "currency", "refunded", "refunded_cents", "created_at", "buyer_currency", "buyer_total_cents", "buyer_refunded_cents", "buyer_fx_rate"}

func writeSalesCSV(w io.Writer, sales []saleListItem) error {
	cw := csv.NewWriter(w)
	if err := writeSalesCSVHeader(cw); err != nil {
		return err
	}
	if err := writeSalesCSVRows(cw, sales); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func streamSalesCSV(w io.Writer, walkPages func(cmdutil.PageVisitor[salesListResponse]) error) error {
	cw := csv.NewWriter(w)
	if err := writeSalesCSVHeader(cw); err != nil {
		return err
	}
	err := walkPages(func(page salesListResponse) (bool, error) {
		return false, writeSalesCSVRows(cw, page.Sales)
	})
	cw.Flush()
	if err != nil {
		return err
	}
	return cw.Error()
}

func writeSalesCSVHeader(cw *csv.Writer) error {
	return cw.Write(salesCSVHeader)
}

func writeSalesCSVRows(cw *csv.Writer, sales []saleListItem) error {
	for _, s := range sales {
		if err := cw.Write([]string{
			s.ID,
			s.Email,
			s.ProductName,
			formatNullableInt(firstNullableInt(s.TotalCents, s.Price)),
			s.csvCurrency(),
			strconv.FormatBool(s.Refunded),
			formatNullableInt(firstNullableInt(s.RefundedCents, s.AmountRefundedCents)),
			s.CreatedAt,
			s.buyerCSVCurrency(),
			s.buyerCSVCents(func(p *buyerPresentment) *nullableInt { return p.TotalCents }),
			s.buyerCSVCents(func(p *buyerPresentment) *nullableInt { return p.RefundedCents }),
			s.buyerCSVFxRate(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s saleListItem) buyerCSVCurrency() string {
	if s.BuyerPresentment == nil {
		return ""
	}
	return s.BuyerPresentment.Currency
}

func (s saleListItem) buyerCSVCents(field func(*buyerPresentment) *nullableInt) string {
	if s.BuyerPresentment == nil {
		return ""
	}
	value := field(s.BuyerPresentment)
	if value == nil || !value.valid {
		return ""
	}
	return strconv.Itoa(int(value.value))
}

func (s saleListItem) buyerCSVFxRate() string {
	if s.BuyerPresentment == nil {
		return ""
	}
	return s.BuyerPresentment.FxRate
}

func (s saleListItem) csvCurrency() string {
	for _, value := range []string{s.Currency, s.CurrencyType, s.PriceCurrencyType} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNullableInt(values ...*nullableInt) *nullableInt {
	for _, value := range values {
		if value != nil && value.valid {
			return value
		}
	}
	return nil
}

func formatNullableInt(value *nullableInt) string {
	if value == nil {
		return "0"
	}
	return strconv.Itoa(int(value.value))
}

func writeSalesTable(w io.Writer, style output.Styler, sales []saleListItem) error {
	tbl := output.NewStyledTable(style, "ID", "EMAIL", "PRODUCT", "TOTAL", "DATE")
	for _, s := range sales {
		id := s.ID
		if s.Refunded {
			id = s.ID + " " + style.Red("(refunded)")
		}
		total := s.FormattedTotal
		if buyerTotal := s.BuyerPresentment.formattedTotal(); buyerTotal != "" {
			total = total + " " + style.Dim("(buyer: "+buyerTotal+")")
		}
		tbl.AddRow(id, s.Email, s.ProductName, total, s.CreatedAt)
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

func salesCSVAllHint(product, email, orderID, before, after string) string {
	return cmdutil.ReplayCommand("gumroad sales list",
		cmdutil.CommandArg{Flag: "--product", Value: product},
		cmdutil.CommandArg{Flag: "--email", Value: email},
		cmdutil.CommandArg{Flag: "--order", Value: orderID},
		cmdutil.CommandArg{Flag: "--before", Value: before},
		cmdutil.CommandArg{Flag: "--after", Value: after},
		cmdutil.CommandArg{Flag: "--all", Boolean: true},
		cmdutil.CommandArg{Flag: "--csv", Boolean: true},
	)
}
