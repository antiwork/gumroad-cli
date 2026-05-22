package sales

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const salesSummaryMaxDateRangeDays = 366

var salesSummaryGroups = map[string]bool{
	"product": true,
	"day":     true,
	"week":    true,
	"month":   true,
}

type salesSummaryResponse struct {
	Success     bool                        `json:"success"`
	GrossCents  api.JSONInt                 `json:"gross_cents"`
	NetCents    api.JSONInt                 `json:"net_cents"`
	Units       api.JSONInt                 `json:"units"`
	RefundCents api.JSONInt                 `json:"refunded_cents"`
	RefundUnits api.JSONInt                 `json:"refunded_units"`
	Currency    string                      `json:"currency"`
	From        string                      `json:"from"`
	To          string                      `json:"to"`
	Breakdown   []salesSummaryBreakdownItem `json:"breakdown,omitempty"`
}

type salesSummaryBreakdownItem struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	GrossCents  api.JSONInt `json:"gross_cents"`
	NetCents    api.JSONInt `json:"net_cents"`
	Units       api.JSONInt `json:"units"`
	RefundCents api.JSONInt `json:"refunded_cents"`
	RefundUnits api.JSONInt `json:"refunded_units"`
}

func newSummaryCmd() *cobra.Command {
	var from, to, groupBy string

	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show aggregated sales totals",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad sales summary
  gumroad sales summary --from 2026-01-01 --to 2026-05-21
  gumroad sales summary --group-by product
  gumroad sales summary --group-by month --from 2025-05-21`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := validateSalesSummaryFlags(c, from, to, groupBy); err != nil {
				return err
			}

			params := url.Values{}
			if from != "" {
				params.Set("from", from)
			}
			if to != "" {
				params.Set("to", to)
			}
			if groupBy != "" {
				params.Set("group_by", groupBy)
			}

			return cmdutil.RunRequestDecoded[salesSummaryResponse](opts, "Fetching sales summary...", http.MethodGet, "/sales/summary", params, func(resp salesSummaryResponse) error {
				return renderSalesSummary(opts, resp, groupBy)
			})
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Summarize sales from date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&to, "to", "", "Summarize sales to date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "Group breakdown by product, day, week, or month")

	return cmd
}

func validateSalesSummaryFlags(cmd *cobra.Command, from, to, groupBy string) error {
	if err := cmdutil.RequireDateFlag(cmd, "from", from); err != nil {
		return err
	}
	if err := cmdutil.RequireDateFlag(cmd, "to", to); err != nil {
		return err
	}
	if cmd.Flags().Changed("group-by") && !salesSummaryGroups[groupBy] {
		return cmdutil.UsageErrorf(cmd, "--group-by must be one of: product, day, week, month")
	}
	if from == "" || to == "" {
		return nil
	}

	fromDate, _ := time.Parse("2006-01-02", from)
	toDate, _ := time.Parse("2006-01-02", to)
	if fromDate.After(toDate) {
		return cmdutil.UsageErrorf(cmd, "--from must be on or before --to")
	}
	if int(toDate.Sub(fromDate).Hours()/24) > salesSummaryMaxDateRangeDays {
		return cmdutil.UsageErrorf(cmd, "date range cannot exceed %d days", salesSummaryMaxDateRangeDays)
	}
	return nil
}

func renderSalesSummary(opts cmdutil.Options, resp salesSummaryResponse, groupBy string) error {
	if opts.PlainOutput {
		return writeSalesSummaryPlain(opts.Out(), resp, groupBy)
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writeSalesSummaryTable(w, style, resp); err != nil {
			return err
		}
		if len(resp.Breakdown) == 0 {
			return nil
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		return writeSalesSummaryBreakdownTable(w, style, resp, groupBy)
	})
}

func writeSalesSummaryTable(w io.Writer, style output.Styler, resp salesSummaryResponse) error {
	tbl := output.NewStyledTable(style, "RANGE", "GROSS", "NET", "UNITS", "REFUNDS", "REFUNDED")
	tbl.AddRow(
		resp.From+".."+resp.To,
		formatSalesSummaryMoney(resp.GrossCents, resp.Currency),
		formatSalesSummaryMoney(resp.NetCents, resp.Currency),
		formatSalesSummaryInt(resp.Units),
		formatSalesSummaryMoney(resp.RefundCents, resp.Currency),
		formatSalesSummaryInt(resp.RefundUnits),
	)
	return tbl.Render(w)
}

func writeSalesSummaryBreakdownTable(w io.Writer, style output.Styler, resp salesSummaryResponse, groupBy string) error {
	if groupBy == "product" {
		tbl := output.NewStyledTable(style, "PRODUCT", "NAME", "GROSS", "NET", "UNITS", "REFUNDS", "REFUNDED")
		for _, item := range resp.Breakdown {
			tbl.AddRow(
				item.Key,
				salesSummaryLabel(item.Label),
				formatSalesSummaryMoney(item.GrossCents, resp.Currency),
				formatSalesSummaryMoney(item.NetCents, resp.Currency),
				formatSalesSummaryInt(item.Units),
				formatSalesSummaryMoney(item.RefundCents, resp.Currency),
				formatSalesSummaryInt(item.RefundUnits),
			)
		}
		return tbl.Render(w)
	}

	tbl := output.NewStyledTable(style, "PERIOD", "GROSS", "NET", "UNITS", "REFUNDS", "REFUNDED")
	for _, item := range resp.Breakdown {
		tbl.AddRow(
			item.Key,
			formatSalesSummaryMoney(item.GrossCents, resp.Currency),
			formatSalesSummaryMoney(item.NetCents, resp.Currency),
			formatSalesSummaryInt(item.Units),
			formatSalesSummaryMoney(item.RefundCents, resp.Currency),
			formatSalesSummaryInt(item.RefundUnits),
		)
	}
	return tbl.Render(w)
}

func writeSalesSummaryPlain(w io.Writer, resp salesSummaryResponse, groupBy string) error {
	rows := [][]string{{
		"summary",
		resp.From + ".." + resp.To,
		"",
		resp.Currency,
		formatSalesSummaryInt(resp.GrossCents),
		formatSalesSummaryInt(resp.NetCents),
		formatSalesSummaryInt(resp.Units),
		formatSalesSummaryInt(resp.RefundCents),
		formatSalesSummaryInt(resp.RefundUnits),
	}}
	for _, item := range resp.Breakdown {
		rows = append(rows, []string{
			groupBy,
			item.Key,
			item.Label,
			resp.Currency,
			formatSalesSummaryInt(item.GrossCents),
			formatSalesSummaryInt(item.NetCents),
			formatSalesSummaryInt(item.Units),
			formatSalesSummaryInt(item.RefundCents),
			formatSalesSummaryInt(item.RefundUnits),
		})
	}
	return output.PrintPlain(w, rows)
}

func salesSummaryLabel(label string) string {
	if label == "" {
		return "-"
	}
	return label
}

func formatSalesSummaryMoney(cents api.JSONInt, currency string) string {
	amount := cmdutil.FormatMoney(int(cents), currency)
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "USD" {
		if strings.HasPrefix(amount, "-") {
			return "-$" + strings.TrimPrefix(amount, "-")
		}
		return "$" + amount
	}
	if code == "" {
		return amount
	}
	return fmt.Sprintf("%s %s", amount, code)
}

func formatSalesSummaryInt(value api.JSONInt) string {
	return strconv.Itoa(int(value))
}
