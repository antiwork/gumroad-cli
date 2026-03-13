package categories

import (
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type categoryListItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type categoriesListResponse struct {
	VariantCategories []categoryListItem `json:"variant_categories"`
}

func newListCmd() *cobra.Command {
	var product string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List variant categories for a product",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			return cmdutil.RunRequestDecoded[categoriesListResponse](opts, "Fetching variant categories...", "GET", cmdutil.JoinPath("products", product, "variant_categories"), url.Values{}, func(resp categoriesListResponse) error {
				if len(resp.VariantCategories) == 0 {
					return cmdutil.PrintInfo(opts, "No variant categories found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, vc := range resp.VariantCategories {
						rows = append(rows, []string{vc.ID, vc.Title})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "TITLE")
				for _, vc := range resp.VariantCategories {
					tbl.AddRow(vc.ID, vc.Title)
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
