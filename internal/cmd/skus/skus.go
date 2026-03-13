package skus

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func NewProductSKUsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "skus <id>",
		Short:   "List SKUs for a product",
		Args:    cmdutil.ExactArgs(1),
		Example: "  gumroad products skus <id>",
		RunE: func(c *cobra.Command, args []string) error {
			return runList(c, args[0])
		},
	}
}

func runList(c *cobra.Command, product string) error {
	opts := cmdutil.OptionsFrom(c)

	return cmdutil.RunRequest(opts, "Fetching SKUs...", "GET", cmdutil.JoinPath("products", product, "skus"), url.Values{}, func(data json.RawMessage) error {
		var resp struct {
			SKUs []struct {
				ID               string      `json:"id"`
				Name             string      `json:"name"`
				PriceDifference  api.JSONInt `json:"price_difference_cents"`
				MaxPurchaseCount api.JSONInt `json:"max_purchase_count"`
			} `json:"skus"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("could not parse response: %w", err)
		}

		if len(resp.SKUs) == 0 {
			return cmdutil.PrintInfo(opts, "No SKUs found.")
		}

		if opts.PlainOutput {
			var rows [][]string
			for _, sku := range resp.SKUs {
				rows = append(rows, []string{
					sku.ID,
					sku.Name,
					fmt.Sprintf("%d", sku.PriceDifference),
					formatSKUMaxPurchases(sku.MaxPurchaseCount),
				})
			}
			return output.PrintPlain(opts.Out(), rows)
		}

		style := opts.Style()
		tbl := output.NewStyledTable(style, "ID", "NAME", "PRICE DIFF (cents)", "MAX PURCHASES")
		for _, sku := range resp.SKUs {
			tbl.AddRow(sku.ID, sku.Name, fmt.Sprintf("%d", sku.PriceDifference), formatSKUMaxPurchases(sku.MaxPurchaseCount))
		}
		return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
			return tbl.Render(w)
		})
	})
}

func formatSKUMaxPurchases(maxPurchaseCount api.JSONInt) string {
	if maxPurchaseCount > 0 {
		return fmt.Sprintf("%d", maxPurchaseCount)
	}
	return "unlimited"
}
