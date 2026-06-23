package upsells

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List upsells and cross-sells",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			return cmdutil.RunRequest(opts, "Fetching upsells...", http.MethodGet, "/upsells", url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					Upsells []upsell `json:"upsells"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				if len(resp.Upsells) == 0 {
					return cmdutil.PrintInfo(opts, "No upsells found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, u := range resp.Upsells {
						rows = append(rows, listRow(u))
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "NAME", "TYPE", "PRODUCT", "DISCOUNT", "PAUSED")
				for _, u := range resp.Upsells {
					tbl.AddRow(listRow(u)...)
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}
}

func listRow(u upsell) []string {
	return []string{
		u.ID,
		u.Name,
		upsellType(u),
		u.Product.Name,
		formatDiscount(u.Discount),
		yesNo(u.Paused),
	}
}
