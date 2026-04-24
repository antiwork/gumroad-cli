package affiliates

import (
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type affiliateListItem struct {
	ID                   string `json:"id"`
	Email                string `json:"email"`
	CommissionPercentage int    `json:"commission_percentage"`
}

type affiliatesListResponse struct {
	Affiliates []affiliateListItem `json:"affiliates"`
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List affiliates",
		Args:    cmdutil.ExactArgs(0),
		Example: `  gumroad affiliates list`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[affiliatesListResponse](opts, "Fetching affiliates...", "GET", "/affiliates", url.Values{}, func(resp affiliatesListResponse) error {
				if len(resp.Affiliates) == 0 {
					return cmdutil.PrintInfo(opts, "No affiliates found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, a := range resp.Affiliates {
						rows = append(rows, []string{a.ID, a.Email, cmdutil.FormatInt(a.CommissionPercentage) + "%"})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "EMAIL", "COMMISSION")
				for _, a := range resp.Affiliates {
					tbl.AddRow(a.ID, a.Email, cmdutil.FormatInt(a.CommissionPercentage)+"%")
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}
}
