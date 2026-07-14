package pages

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview [path]",
		Short: "Preview server sanitization for a page's custom HTML",
		Long:  "Preview what a page's custom HTML looks like after server sanitization, without publishing anything. The dry run is page-agnostic — iterate here, then publish with `gumroad pages push <slug>`.",
		Args:  pageHTMLArgs,
		Example: `  gumroad pages preview ./about.html
  gumroad pages preview - < about.html`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			input, err := pageutil.ReadHTML(opts.In(), path)
			if err != nil {
				return cmdutil.UsageErrorf(c, "%s", err)
			}

			target := pageutil.ProfileTarget()
			err = cmdutil.RunRequestDecoded[pageutil.PreviewResponse](
				opts,
				"Previewing page...",
				http.MethodPost,
				target.PreviewPath,
				pageutil.HTMLParams(input.HTML),
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
			return pageutil.TranslateRateLimitError(err, pageutil.PagesPreviewRateLimitMessage)
		},
	}
}
