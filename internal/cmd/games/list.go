package games

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var product string
	c := &cobra.Command{
		Use:   "list",
		Short: "List a product's game builds",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.UsageErrorf(c, "--product is required")
			}
			return cmdutil.RunRequestDecoded[buildsResponse](opts, "Fetching builds...", "GET", buildsPath(product), url.Values{}, func(resp buildsResponse) error {
				if len(resp.Builds) == 0 {
					return cmdutil.PrintInfo(opts, "No game builds found.")
				}
				if opts.PlainOutput {
					var rows [][]string
					for _, b := range resp.Builds {
						rows = append(rows, []string{b.ID, itoa(b.Number), b.Status, uploadui.HumanBytes(b.ByteSize)})
					}
					return output.PrintPlain(opts.Out(), rows)
				}
				tbl := output.NewStyledTable(opts.Style(), "ID", "BUILD", "STATUS", "SIZE")
				for _, b := range resp.Builds {
					tbl.AddRow(b.ID, itoa(b.Number), b.Status, uploadui.HumanBytes(b.ByteSize))
				}
				return tbl.Render(opts.Out())
			})
		},
	}
	c.Flags().StringVar(&product, "product", "", "Product ID or permalink (required)")
	return c
}
