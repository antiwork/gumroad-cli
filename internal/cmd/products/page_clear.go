package products

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear <product_id>",
		Short: "Clear a product landing page",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID := args[0]
			ok, err := cmdutil.ConfirmAction(opts, "Clear landing page for product "+productID+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "clear landing page for product "+productID, productID)
			}

			target := pageutil.ProductTarget(productID)
			err = cmdutil.RunRequestDecoded[pageutil.UpdateResponse](
				opts,
				"Clearing page...",
				http.MethodPut,
				target.Path,
				pageutil.ClearParams(),
				func(resp pageutil.UpdateResponse) error {
					return pageutil.RenderSanitizationResult(opts, pageutil.RenderResult{
						Action:       "Cleared page",
						BeforeHTML:   pageutil.PreviousHTML(resp),
						AfterHTML:    resp.Product.CustomHTML,
						LandingURL:   pageutil.LandingURL(resp.Product),
						Report:       resp.SanitizationReport,
						ClearMessage: "Page cleared.",
					})
				},
			)
			return pageutil.TranslateRateLimitError(err, pageutil.PublishRateLimitMessage)
		},
	}
}
