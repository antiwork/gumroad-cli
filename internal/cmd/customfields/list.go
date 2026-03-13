package customfields

import (
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type customFieldListItem struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Type     string `json:"type"`
}

type customFieldsListResponse struct {
	CustomFields []customFieldListItem `json:"custom_fields"`
}

func newListCmd() *cobra.Command {
	var product string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom fields for a product",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			return cmdutil.RunRequestDecoded[customFieldsListResponse](opts, "Fetching custom fields...", "GET", cmdutil.JoinPath("products", product, "custom_fields"), url.Values{}, func(resp customFieldsListResponse) error {
				if len(resp.CustomFields) == 0 {
					return cmdutil.PrintInfo(opts, "No custom fields found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, cf := range resp.CustomFields {
						rows = append(rows, []string{cf.Name, fmt.Sprintf("required=%v", cf.Required), cf.Type})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "NAME", "REQUIRED", "TYPE")
				for _, cf := range resp.CustomFields {
					required := "no"
					if cf.Required {
						required = style.Green("yes")
					}
					tbl.AddRow(cf.Name, required, cf.Type)
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
