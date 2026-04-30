package discover

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/publiccmd"
	"github.com/spf13/cobra"
)

const (
	defaultLimit   = 30
	maxLimit       = 500
	maxNameWidth   = 50
	centsPerDollar = 100
	searchPath     = "/products/search.json"
)

var allowedSorts = []string{
	"default",
	"price_asc",
	"price_desc",
	"hot_and_new",
	"most_reviewed",
	"highest_rated",
	"curated",
}

type searchProduct struct {
	ID                string        `json:"id"`
	Permalink         string        `json:"permalink"`
	Name              string        `json:"name"`
	Seller            searchSeller  `json:"seller"`
	Ratings           searchRatings `json:"ratings"`
	NativeType        string        `json:"native_type"`
	PriceCents        int64         `json:"price_cents"`
	CurrencyCode      string        `json:"currency_code"`
	IsPayWhatYouWant  bool          `json:"is_pay_what_you_want"`
	URL               string        `json:"url"`
	ThumbnailURL      string        `json:"thumbnail_url"`
	Recurrence        string        `json:"recurrence"`
	DurationInMonths  *int          `json:"duration_in_months"`
	QuantityRemaining *int64        `json:"quantity_remaining"`
	IsSalesLimited    bool          `json:"is_sales_limited"`
	Description       string        `json:"description"`
}

type searchSeller struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AvatarURL  string `json:"avatar_url"`
	ProfileURL string `json:"profile_url"`
	IsVerified bool   `json:"is_verified"`
}

type searchRatings struct {
	Count   int     `json:"count"`
	Average float64 `json:"average"`
}

type searchResponse struct {
	Total    int             `json:"total"`
	Products []searchProduct `json:"products"`
}

func newSearchCmd() *cobra.Command {
	var (
		tag      string
		minPrice int
		maxPrice int
		sort     string
		limit    int
		from     int
	)

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search public Gumroad products",
		Long: `Search the public Gumroad catalog. The query is optional — omit it to browse
trending picks. Filters and sort match the gumroad.com/discover surface.`,
		Example: `  gumroad discover search "machine learning"
  gumroad discover search --tag font --sort price_asc --limit 50
  gumroad discover search "design" --max-price 25 --json
  gumroad discover search --tag productivity --plain`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			if !sortAllowed(sort) {
				return cmdutil.NewUsageError(c, fmt.Sprintf("invalid --sort %q (allowed: %s)", sort, strings.Join(allowedSorts, ", ")))
			}
			if limit < 1 {
				return cmdutil.NewUsageError(c, "--limit must be at least 1")
			}
			if limit > maxLimit {
				return cmdutil.NewUsageError(c, fmt.Sprintf("--limit must not exceed %d", maxLimit))
			}
			if from < 0 {
				return cmdutil.NewUsageError(c, "--from must not be negative")
			}
			if minPrice < 0 || maxPrice < 0 {
				return cmdutil.NewUsageError(c, "price filters must not be negative")
			}
			if maxPrice > 0 && minPrice > maxPrice {
				return cmdutil.NewUsageError(c, "--min-price cannot exceed --max-price")
			}

			params := url.Values{}
			if len(args) == 1 && args[0] != "" {
				params.Set("query", args[0])
			}
			if tag != "" {
				params.Set("tags", tag)
			}
			if minPrice > 0 {
				params.Set("min_price", strconv.Itoa(minPrice))
			}
			if maxPrice > 0 {
				params.Set("max_price", strconv.Itoa(maxPrice))
			}
			if sort != "default" {
				params.Set("sort", sort)
			}
			params.Set("size", strconv.Itoa(limit))
			if from > 0 {
				params.Set("from", strconv.Itoa(from))
			}

			return publiccmd.RunGetDecoded[searchResponse](opts, "Searching products...", searchPath, params, func(resp searchResponse) error {
				if len(resp.Products) == 0 {
					return cmdutil.PrintInfo(opts, "No products found.")
				}

				if opts.PlainOutput {
					rows := make([][]string, 0, len(resp.Products))
					for _, p := range resp.Products {
						rows = append(rows, []string{p.Name, p.Seller.Name, formatPrice(p), formatRating(p.Ratings), p.URL})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					tbl := output.NewStyledTable(style, "NAME", "SELLER", "PRICE", "RATING", "URL")
					for _, p := range resp.Products {
						tbl.AddRow(truncate(p.Name, maxNameWidth), p.Seller.Name, formatPrice(p), formatRating(p.Ratings), p.URL)
					}
					if err := tbl.Render(w); err != nil {
						return err
					}
					if !opts.Quiet {
						summary := fmt.Sprintf("\nShowing %d of %d", len(resp.Products), resp.Total)
						return output.Writeln(w, style.Dim(summary))
					}
					return nil
				})
			})
		},
	}

	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag (e.g. design, productivity)")
	cmd.Flags().IntVar(&minPrice, "min-price", 0, "Minimum price in dollars")
	cmd.Flags().IntVar(&maxPrice, "max-price", 0, "Maximum price in dollars")
	cmd.Flags().StringVar(&sort, "sort", "default", "Sort order: "+strings.Join(allowedSorts, ", "))
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Number of results to return (max 500)")
	cmd.Flags().IntVar(&from, "from", 0, "Offset for pagination")

	return cmd
}

func sortAllowed(s string) bool {
	for _, allowed := range allowedSorts {
		if s == allowed {
			return true
		}
	}
	return false
}

func formatPrice(p searchProduct) string {
	if p.IsPayWhatYouWant {
		return "PWYW"
	}
	dollars := float64(p.PriceCents) / centsPerDollar
	if p.PriceCents == 0 {
		return "Free"
	}
	currency := strings.ToUpper(p.CurrencyCode)
	if currency == "" {
		currency = "USD"
	}
	if currency == "USD" {
		if p.Recurrence != "" {
			return fmt.Sprintf("$%.2f / %s", dollars, p.Recurrence)
		}
		return fmt.Sprintf("$%.2f", dollars)
	}
	if p.Recurrence != "" {
		return fmt.Sprintf("%.2f %s / %s", dollars, currency, p.Recurrence)
	}
	return fmt.Sprintf("%.2f %s", dollars, currency)
}

func formatRating(r searchRatings) string {
	if r.Count == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f (%d)", r.Average, r.Count)
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "…"
}
