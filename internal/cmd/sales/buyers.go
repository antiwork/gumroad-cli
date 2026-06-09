package sales

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type buyersSaleItem struct {
	Email     string `json:"email"`
	FullName  string `json:"full_name"`
	CreatedAt string `json:"created_at"`
}

type buyersSalesPage struct {
	Success     bool             `json:"success"`
	Sales       []buyersSaleItem `json:"sales"`
	NextPageKey string           `json:"next_page_key,omitempty"`
}

type buyer struct {
	Email            string `json:"email"`
	Name             string `json:"name"`
	PurchaseCount    int    `json:"purchase_count"`
	LastPurchaseDate string `json:"last_purchase_date"`
}

type buyersResponse struct {
	Success bool    `json:"success"`
	Buyers  []buyer `json:"buyers"`
}

func (b buyer) fields() []string {
	return []string{b.Email, b.Name, strconv.Itoa(b.PurchaseCount), b.LastPurchaseDate}
}

func newBuyersCmd() *cobra.Command {
	var products []string
	var before, after string
	var csvOutput bool

	cmd := &cobra.Command{
		Use:   "buyers",
		Short: "List deduplicated buyers for a product",
		Args:  cmdutil.ExactArgs(0),
		Long: `List the unique buyers who purchased one or more products.

Buyers are deduplicated by email and aggregated across every page, so a buyer
who purchased more than once appears a single time with a purchase count and
their most recent purchase date. Pass --product more than once to union buyers
across a relaunched listing's old and new IDs and dedupe them in one shot.`,
		Example: `  gumroad sales buyers --product <id>
  gumroad sales buyers --product <old-id> --product <new-id>
  gumroad sales buyers --product <id> --after 2024-01-01 --csv
  gumroad sales buyers --product <id> --json --jq '.buyers[].email'`,
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

			return runBuyers(opts, dedupeProducts(products), before, after, csvOutput)
		},
	}

	cmd.Flags().StringArrayVar(&products, "product", nil, "Filter by product ID (repeatable)")
	cmd.Flags().StringVar(&before, "before", "", "Filter sales before date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&after, "after", "", "Filter sales after date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&csvOutput, "csv", false, "Output as CSV")

	return cmd
}

func dedupeProducts(products []string) []string {
	seen := make(map[string]struct{}, len(products))
	var unique []string
	for _, product := range products {
		if product == "" {
			continue
		}
		if _, ok := seen[product]; ok {
			continue
		}
		seen[product] = struct{}{}
		unique = append(unique, product)
	}
	return unique
}

func runBuyers(opts cmdutil.Options, products []string, before, after string, csvOutput bool) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	sp := output.NewSpinnerTo("Fetching buyers...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, token)
	index := newBuyerIndex()

	queries := buyerQueries(products, before, after)
	for _, params := range queries {
		if err := walkBuyerPages(opts, client, params, index.add); err != nil {
			return err
		}
	}

	sp.Stop()
	return renderBuyers(opts, index.sorted(), csvOutput)
}

func buyerQueries(products []string, before, after string) []url.Values {
	base := url.Values{}
	if before != "" {
		base.Set("before", before)
	}
	if after != "" {
		base.Set("after", after)
	}

	if len(products) == 0 {
		return []url.Values{base}
	}

	queries := make([]url.Values, 0, len(products))
	for _, product := range products {
		params := cmdutil.CloneValues(base)
		params.Set("product_id", product)
		queries = append(queries, params)
	}
	return queries
}

func walkBuyerPages(opts cmdutil.Options, client *api.Client, params url.Values, visit func(buyersSaleItem)) error {
	return cmdutil.WalkPagesWithDelay[buyersSalesPage](opts.Context, opts.PageDelay, client, "/sales", params, func(page buyersSalesPage) string {
		return page.NextPageKey
	}, func(page buyersSalesPage) (bool, error) {
		for _, sale := range page.Sales {
			visit(sale)
		}
		return false, nil
	})
}

type buyerIndex struct {
	byEmail map[string]*buyerAggregate
}

type buyerAggregate struct {
	email            string
	name             string
	nameDate         string
	count            int
	lastPurchaseDate string
}

func newBuyerIndex() *buyerIndex {
	return &buyerIndex{byEmail: make(map[string]*buyerAggregate)}
}

func (i *buyerIndex) add(sale buyersSaleItem) {
	email := strings.TrimSpace(sale.Email)
	if email == "" {
		return
	}

	key := strings.ToLower(email)
	aggregate, ok := i.byEmail[key]
	if !ok {
		aggregate = &buyerAggregate{email: email}
		i.byEmail[key] = aggregate
	}

	aggregate.count++
	if sale.CreatedAt > aggregate.lastPurchaseDate {
		aggregate.lastPurchaseDate = sale.CreatedAt
	}

	name := strings.TrimSpace(sale.FullName)
	if name != "" && (aggregate.name == "" || sale.CreatedAt > aggregate.nameDate) {
		aggregate.name = name
		aggregate.nameDate = sale.CreatedAt
	}
}

func (i *buyerIndex) sorted() []buyer {
	buyers := make([]buyer, 0, len(i.byEmail))
	for _, aggregate := range i.byEmail {
		buyers = append(buyers, buyer{
			Email:            aggregate.email,
			Name:             aggregate.name,
			PurchaseCount:    aggregate.count,
			LastPurchaseDate: aggregate.lastPurchaseDate,
		})
	}

	sort.Slice(buyers, func(a, b int) bool {
		if buyers[a].LastPurchaseDate != buyers[b].LastPurchaseDate {
			return buyers[a].LastPurchaseDate > buyers[b].LastPurchaseDate
		}
		return buyers[a].Email < buyers[b].Email
	})
	return buyers
}

func renderBuyers(opts cmdutil.Options, buyers []buyer, csvOutput bool) error {
	if opts.UsesJSONOutput() {
		return printBuyersJSON(opts, buyers)
	}
	if csvOutput {
		return writeBuyersCSV(opts.Out(), buyers)
	}
	if opts.PlainOutput {
		return writeBuyersPlain(opts.Out(), buyers)
	}
	if len(buyers) == 0 {
		return cmdutil.PrintInfo(opts, "No buyers found.")
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		return writeBuyersTable(w, style, buyers)
	})
}

func printBuyersJSON(opts cmdutil.Options, buyers []buyer) error {
	data, err := json.Marshal(buyersResponse{Success: true, Buyers: buyers})
	if err != nil {
		return fmt.Errorf("could not encode JSON output: %w", err)
	}
	return output.PrintJSON(opts.Out(), data, opts.JQExpr)
}

var buyersCSVHeader = []string{"email", "name", "purchase_count", "last_purchase_date"}

func writeBuyersCSV(w io.Writer, buyers []buyer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(buyersCSVHeader); err != nil {
		return err
	}
	for _, b := range buyers {
		if err := cw.Write(b.fields()); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeBuyersPlain(w io.Writer, buyers []buyer) error {
	var rows [][]string
	for _, b := range buyers {
		rows = append(rows, b.fields())
	}
	return output.PrintPlain(w, rows)
}

func writeBuyersTable(w io.Writer, style output.Styler, buyers []buyer) error {
	tbl := output.NewStyledTable(style, "EMAIL", "NAME", "PURCHASES", "LAST PURCHASE")
	for _, b := range buyers {
		tbl.AddRow(b.fields()...)
	}
	return tbl.Render(w)
}
