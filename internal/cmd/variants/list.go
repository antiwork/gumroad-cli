package variants

import (
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type variantListItem struct {
	ID                   string      `json:"id"`
	Name                 string      `json:"name"`
	PriceDifferenceCents api.JSONInt `json:"price_difference_cents"`
	MaxPurchaseCount     api.JSONInt `json:"max_purchase_count"`
}

type variantsListResponse struct {
	Variants []variantListItem `json:"variants"`
}

func newListCmd() *cobra.Command {
	var product, category string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List variants for a category",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if category == "" {
				return cmdutil.MissingFlagError(c, "--category")
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants")
			return cmdutil.RunRequestDecoded[variantsListResponse](opts, "Fetching variants...", "GET", path, url.Values{}, func(resp variantsListResponse) error {
				if len(resp.Variants) == 0 {
					return cmdutil.PrintInfo(opts, "No variants found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, v := range resp.Variants {
						rows = append(rows, []string{
							v.ID,
							v.Name,
							fmt.Sprintf("%d", v.PriceDifferenceCents),
							formatVariantMaxPurchases(v.MaxPurchaseCount),
						})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "NAME", "PRICE DIFF (cents)", "MAX PURCHASES")
				for _, v := range resp.Variants {
					tbl.AddRow(v.ID, v.Name, fmt.Sprintf("%d", v.PriceDifferenceCents), formatVariantMaxPurchases(v.MaxPurchaseCount))
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")

	return cmd
}

func formatVariantMaxPurchases(maxPurchaseCount api.JSONInt) string {
	if maxPurchaseCount > 0 {
		return fmt.Sprintf("%d", maxPurchaseCount)
	}
	return "unlimited"
}
