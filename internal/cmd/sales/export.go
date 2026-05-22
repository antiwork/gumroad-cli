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
	var from, to, after, before, product string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Request an emailed sales CSV export",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad sales export --from 2026-01-01 --to 2026-05-21
  gumroad sales export --after 2026-01-01 --before 2026-05-21
  gumroad sales export --product <id>
  gumroad sales export --json`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			startDate, err := salesExportDateValue(c, "from", from, "after", after)
			if err != nil {
				return err
			}
			endDate, err := salesExportDateValue(c, "to", to, "before", before)
			if err != nil {
				return err
			}

			params := url.Values{}
			if startDate != "" {
				params.Set("from", startDate)
			}
			if endDate != "" {
				params.Set("to", endDate)
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
	cmd.Flags().StringVar(&after, "after", "", "Alias for --from")
	cmd.Flags().StringVar(&before, "before", "", "Alias for --to")
	cmd.Flags().StringVar(&product, "product", "", "Filter by product ID")

	return cmd
}

func salesExportDateValue(cmd *cobra.Command, primaryFlag, primaryValue, aliasFlag, aliasValue string) (string, error) {
	primaryChanged := cmd.Flags().Changed(primaryFlag)
	aliasChanged := cmd.Flags().Changed(aliasFlag)
	if primaryChanged && aliasChanged {
		return "", cmdutil.UsageErrorf(cmd, "--%s cannot be combined with --%s", aliasFlag, primaryFlag)
	}
	if err := cmdutil.RequireDateFlag(cmd, primaryFlag, primaryValue); err != nil {
		return "", err
	}
	if err := cmdutil.RequireDateFlag(cmd, aliasFlag, aliasValue); err != nil {
		return "", err
	}
	if aliasChanged {
		return aliasValue, nil
	}
	return primaryValue, nil
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
