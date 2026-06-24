package user

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPagePreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview [path]",
		Short: "Preview server sanitization for your profile landing page",
		Args:  profilePageHTMLArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			input, err := pageutil.ReadHTML(opts.In(), profilePageHTMLPath(args))
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
			return pageutil.TranslateRateLimitError(err, pageutil.ProfilePreviewRateLimitMessage)
		},
	}
}
