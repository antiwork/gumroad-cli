package products

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPagePreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview <product_id> [path]",
		Short: "Preview server sanitization for a product landing page",
		Long:  "Preview what a product landing page's HTML looks like after server sanitization, without publishing anything.\n\n" + pageutil.CustomHTMLLimitHelp,
		Args:  productPageHTMLArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			input, err := pageutil.ReadHTML(opts.In(), productPageHTMLPath(args))
			if err != nil {
				return cmdutil.UsageErrorf(c, "%s", err)
			}
			params := pageutil.HTMLParams(input.HTML)
			if err := pageutil.ValidateCustomHTML(input, params); err != nil {
				return cmdutil.InvalidInputErrorf("%s", err)
			}

			target := pageutil.ProductTarget(args[0])
			err = cmdutil.RunRequestDecoded[pageutil.PreviewResponse](
				opts,
				"Previewing page...",
				http.MethodPost,
				target.PreviewPath,
				params,
				func(resp pageutil.PreviewResponse) error {
					return pageutil.RenderSanitizationResult(opts, pageutil.RenderResult{
						Action:     "Previewed page",
						Source:     input.Source,
						BeforeHTML: input.HTML,
						AfterHTML:  resp.CustomHTML,
						Report:     resp.SanitizationReport,
					})
				},
			)
			return pageutil.TranslateRateLimitError(err, pageutil.PreviewRateLimitMessage)
		},
	}
}
