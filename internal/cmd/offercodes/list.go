package offercodes

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

func newListCmd() *cobra.Command {
	var product string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List offer codes for a product",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			return cmdutil.RunRequest(opts, "Fetching offer codes...", "GET", cmdutil.JoinPath("products", product, "offer_codes"), url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					OfferCodes []struct {
						ID               string      `json:"id"`
						Name             string      `json:"name"`
						AmountOff        api.JSONInt `json:"amount_off"`
						PercentOff       api.JSONInt `json:"percent_off"`
						MaxPurchaseCount api.JSONInt `json:"max_purchase_count"`
						Universal        bool        `json:"universal"`
					} `json:"offer_codes"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				if len(resp.OfferCodes) == 0 {
					return cmdutil.PrintInfo(opts, "No offer codes found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, oc := range resp.OfferCodes {
						rows = append(rows, []string{
							oc.ID,
							oc.Name,
							formatOfferCodeDiscount(oc.AmountOff, oc.PercentOff),
							formatOfferCodeMaxUses(oc.MaxPurchaseCount),
							formatOfferCodeUniversal(oc.Universal),
						})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "NAME", "DISCOUNT", "MAX USES", "UNIVERSAL")
				for _, oc := range resp.OfferCodes {
					tbl.AddRow(
						oc.ID,
						oc.Name,
						formatOfferCodeDiscount(oc.AmountOff, oc.PercentOff),
						formatOfferCodeMaxUses(oc.MaxPurchaseCount),
						formatOfferCodeUniversal(oc.Universal),
					)
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")

	return cmd
}

func formatOfferCodeDiscount(amountOff, percentOff api.JSONInt) string {
	if amountOff > 0 {
		return fmt.Sprintf("%d cents off", amountOff)
	}
	return fmt.Sprintf("%d%%", percentOff)
}

func formatOfferCodeMaxUses(maxPurchaseCount api.JSONInt) string {
	if maxPurchaseCount > 0 {
		return fmt.Sprintf("%d", maxPurchaseCount)
	}
	return "unlimited"
}

func formatOfferCodeUniversal(universal bool) string {
	if universal {
		return "yes"
	}
	return "no"
}
