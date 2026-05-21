package sales

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type salesExportResponse struct {
	Success        bool   `json:"success"`
	Status         string `json:"status"`
	RecipientEmail string `json:"recipient_email"`
}

func newExportCmd() *cobra.Command {
	var from, to, product string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Request an emailed sales CSV export",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad sales export --from 2026-01-01 --to 2026-05-21
  gumroad sales export --product <id>
  gumroad sales export --json`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireDateFlag(c, "from", from); err != nil {
				return err
			}
			if err := cmdutil.RequireDateFlag(c, "to", to); err != nil {
				return err
			}

			params := url.Values{}
			if from != "" {
				params.Set("from", from)
			}
			if to != "" {
				params.Set("to", to)
			}
			if product != "" {
				params.Set("product_id", product)
			}

			return cmdutil.RunRequestDecoded[salesExportResponse](opts, "Requesting sales export...", http.MethodPost, "/sales/exports", params, func(resp salesExportResponse) error {
				return renderSalesExport(opts, resp)
			})
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Export sales from date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&to, "to", "", "Export sales to date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&product, "product", "", "Filter by product ID")

	return cmd
}

func renderSalesExport(opts cmdutil.Options, resp salesExportResponse) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{resp.Status, resp.RecipientEmail}})
	}
	return cmdutil.PrintSuccess(opts, salesExportQueuedMessage(resp.RecipientEmail))
}

func salesExportQueuedMessage(email string) string {
	if email == "" {
		return "Sales export queued."
	}
	return fmt.Sprintf("CSV will be emailed to %s when ready.", email)
}
