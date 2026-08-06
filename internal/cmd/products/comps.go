package products

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type compsExample struct {
	Name  string `json:"name"`
	Price string `json:"price"`
	URL   string `json:"url"`
}

type compsPriceCents struct {
	P25 *int64 `json:"p25"`
	P50 *int64 `json:"p50"`
	P75 *int64 `json:"p75"`
}

type compsData struct {
	Count      int64           `json:"count"`
	Currency   string          `json:"currency,omitempty"`
	PriceCents compsPriceCents `json:"price_cents"`
	Examples   []compsExample  `json:"examples"`
}

type compsResponse struct {
	Success bool      `json:"success"`
	Comps   compsData `json:"comps"`
}

func newCompsCmd() *cobra.Command {
	var category string
	var query string
	var currency string

	cmd := &cobra.Command{
		Use:   "comps",
		Short: "See what similar products charge",
		Long: "Report the price distribution of comparable priced public products on Gumroad, " +
			"so a new product can be priced against real marketplace numbers. " +
			"Categories include their descendants, and free listings are excluded. " +
			"Find category paths with gumroad products categories.",
		Args: cmdutil.ExactArgs(0),
		Example: `  gumroad products comps --category design/ui-and-web/figma
  gumroad products comps --category music-and-sound-design --query "whoosh sfx"
  gumroad products comps --query "notion template" --json --jq '.comps.price_cents'`,
		RunE: func(c *cobra.Command, args []string) error {
			category = strings.TrimSpace(category)
			query = strings.TrimSpace(query)
			if category == "" && query == "" {
				return cmdutil.UsageErrorf(c, "at least one of --category or --query is required")
			}
			if currency != "" && !cmdutil.IsSupportedCurrency(currency) {
				return cmdutil.UsageErrorf(c, "unsupported currency %q", currency)
			}
			opts := cmdutil.OptionsFrom(c)

			params := url.Values{}
			if category != "" {
				params.Set("taxonomy", category)
			}
			if query != "" {
				params.Set("query", query)
			}
			if currency != "" {
				params.Set("price_currency_type", strings.ToLower(currency))
			}

			return cmdutil.RunRequestDecoded[compsResponse](opts, "Fetching comparable products...", "GET", "/products/comps", params, func(resp compsResponse) error {
				return renderComps(opts, resp.Comps)
			})
		},
	}

	cmd.Flags().StringVar(&category, "category", "", "Category path, descendants included (see: gumroad products categories)")
	cmd.Flags().StringVar(&query, "query", "", "Full-text filter over product names and descriptions")
	cmd.Flags().StringVar(&currency, "currency", "", "Currency to compare in (ISO code, default usd)")

	return cmd
}

func renderComps(opts cmdutil.Options, comps compsData) error {
	if comps.Count == 0 {
		return cmdutil.PrintInfo(opts, "No comparable products found.")
	}

	if opts.PlainOutput {
		rows := [][]string{
			{"count", fmt.Sprintf("%d", comps.Count)},
			{"currency", compsCurrency(comps)},
			{"p25", centsString(comps.PriceCents.P25)},
			{"p50", centsString(comps.PriceCents.P50)},
			{"p75", centsString(comps.PriceCents.P75)},
		}
		for _, example := range comps.Examples {
			rows = append(rows, []string{"example", example.Name, example.Price, example.URL})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, fmt.Sprintf("%s comparable products", style.Bold(fmt.Sprintf("%d", comps.Count)))); err != nil {
			return err
		}
		cur := compsCurrency(comps)
		if err := output.Writeln(w, fmt.Sprintf("Prices (%s): 25th %s · median %s · 75th %s",
			strings.ToUpper(cur),
			compsMoneyString(comps.PriceCents.P25, cur),
			style.Bold(compsMoneyString(comps.PriceCents.P50, cur)),
			compsMoneyString(comps.PriceCents.P75, cur))); err != nil {
			return err
		}
		if len(comps.Examples) == 0 {
			return nil
		}
		if err := output.Writeln(w, "\n"+style.Bold("Top sellers")); err != nil {
			return err
		}
		tbl := output.NewStyledTable(style, "NAME", "PRICE", "URL")
		for _, example := range comps.Examples {
			tbl.AddRow(example.Name, example.Price, example.URL)
		}
		return tbl.Render(w)
	})
}

// compsCurrency defaults to usd for servers that predate the currency field.
func compsCurrency(comps compsData) string {
	if comps.Currency == "" {
		return "usd"
	}
	return strings.ToLower(comps.Currency)
}

func centsString(cents *int64) string {
	if cents == nil {
		return ""
	}
	return fmt.Sprintf("%d", *cents)
}

func compsMoneyString(cents *int64, currency string) string {
	if cents == nil {
		return "n/a"
	}
	formatted := cmdutil.FormatMoney(int(*cents), currency)
	formatted = strings.TrimSuffix(formatted, ".00")
	if strings.ToLower(currency) == "usd" {
		return "$" + formatted
	}
	return formatted
}
