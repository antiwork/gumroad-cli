package products

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPagePublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <product_id> [path]",
		Short: "Publish custom HTML for a product landing page",
		Args:  productPageHTMLArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			input, err := pageutil.ReadHTML(opts.In(), productPageHTMLPath(args))
			if err != nil {
				return cmdutil.UsageErrorf(c, "%s", err)
			}

			target := pageutil.ProductTarget(args[0])
			err = cmdutil.RunRequestDecoded[pageutil.UpdateResponse](
				opts,
				"Publishing page...",
				http.MethodPut,
				target.Path,
				pageutil.HTMLParams(input.HTML),
				func(resp pageutil.UpdateResponse) error {
					return pageutil.RenderSanitizationResult(opts, pageutil.RenderResult{
						Action:     "Published page",
						Source:     input.Source,
						BeforeHTML: input.HTML,
						AfterHTML:  resp.Product.CustomHTML,
						LandingURL: pageutil.LandingURL(resp.Product),
						Report:     resp.SanitizationReport,
					})
				},
			)
			return pageutil.TranslateRateLimitError(err, pageutil.PublishRateLimitMessage)
		},
	}
}
