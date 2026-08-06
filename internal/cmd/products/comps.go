package products

import (
	"fmt"
	"io"
	"net/url"

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

	cmd := &cobra.Command{
		Use:   "comps",
		Short: "See what similar products charge",
		Long: "Report the price distribution of comparable public products on Gumroad, " +
			"so a new product can be priced against real marketplace numbers. " +
			"Find category paths with  gumroad products categories.",
		Args: cmdutil.ExactArgs(0),
		Example: `  gumroad products comps --category design/ui-and-web/figma
  gumroad products comps --category music-and-sound-design --query "whoosh sfx"
  gumroad products comps --query "notion template" --json --jq '.comps.price_cents'`,
		RunE: func(c *cobra.Command, args []string) error {
			if category == "" && query == "" {
				return fmt.Errorf("at least one of --category or --query is required")
			}
			opts := cmdutil.OptionsFrom(c)

			params := url.Values{}
			if category != "" {
				params.Set("taxonomy", category)
			}
			if query != "" {
				params.Set("query", query)
			}

			return cmdutil.RunRequestDecoded[compsResponse](opts, "Fetching comparable products...", "GET", "/products/comps", params, func(resp compsResponse) error {
				return renderComps(opts, resp.Comps)
			})
		},
	}

	cmd.Flags().StringVar(&category, "category", "", "Category path (see: gumroad products categories)")
	cmd.Flags().StringVar(&query, "query", "", "Full-text filter over product names and descriptions")

	return cmd
}

func renderComps(opts cmdutil.Options, comps compsData) error {
	if comps.Count == 0 {
		return cmdutil.PrintInfo(opts, "No comparable products found.")
	}

	if opts.PlainOutput {
		rows := [][]string{
			{"count", fmt.Sprintf("%d", comps.Count)},
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
		if err := output.Writeln(w, fmt.Sprintf("Prices: 25th %s · median %s · 75th %s",
			centsDollarString(comps.PriceCents.P25),
			style.Bold(centsDollarString(comps.PriceCents.P50)),
			centsDollarString(comps.PriceCents.P75))); err != nil {
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

func centsString(cents *int64) string {
	if cents == nil {
		return ""
	}
	return fmt.Sprintf("%d", *cents)
}

func centsDollarString(cents *int64) string {
	if cents == nil {
		return "n/a"
	}
	if *cents%100 == 0 {
		return fmt.Sprintf("$%d", *cents/100)
	}
	return fmt.Sprintf("$%.2f", float64(*cents)/100)
}
