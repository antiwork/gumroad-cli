package user

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPagePublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish [path]",
		Short: "Publish custom HTML for your profile landing page",
		Args:  profilePageHTMLArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			input, err := pageutil.ReadHTML(opts.In(), profilePageHTMLPath(args))
			if err != nil {
				return cmdutil.UsageErrorf(c, "%s", err)
			}

			target := pageutil.ProfileTarget()
			err = cmdutil.RunRequestDecoded[pageutil.ProfileUpdateResponse](
				opts,
				"Publishing page...",
				http.MethodPut,
				target.Path,
				pageutil.HTMLParams(input.HTML),
				func(resp pageutil.ProfileUpdateResponse) error {
					return pageutil.RenderSanitizationResult(opts, pageutil.RenderResult{
						Action:     "Published page",
						Source:     input.Source,
						BeforeHTML: input.HTML,
						AfterHTML:  resp.CustomHTML,
						LandingURL: resp.ProfileURL,
						Report:     resp.SanitizationReport,
					})
				},
			)
			return pageutil.TranslateRateLimitError(err, pageutil.ProfilePublishRateLimitMessage)
		},
	}
}
