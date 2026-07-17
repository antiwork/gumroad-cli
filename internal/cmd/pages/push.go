package pages

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <slug> <path>",
		Short: "Publish custom HTML to a storefront page",
		Long:  "Publish custom HTML to a storefront page, replacing whatever the page had before.\n\nUse the slug \"profile\" to replace your profile landing page (your store's home page) — that goes through the profile endpoints, same as `gumroad user page publish`.",
		Args:  pushArgs,
		Example: `  gumroad pages push about ./about.html
  gumroad pages push about - < about.html
  gumroad pages push profile ./landing.html`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			slug := args[0]
			input, err := pageutil.ReadHTML(opts.In(), args[1])
			if err != nil {
				return cmdutil.UsageErrorf(c, "%s", err)
			}

			// The profile landing page is not addressable on the /pages API;
			// it keeps its dedicated custom HTML endpoints, which also return
			// the sanitization report the render below shows.
			if slug == profileSlug {
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
				return pageutil.TranslateRateLimitError(err, pageutil.PagesPublishRateLimitMessage)
			}

			err = cmdutil.RunRequestDecoded[pageResponse](
				opts,
				"Publishing page...",
				http.MethodPut,
				pagePath(slug),
				pageutil.HTMLParams(input.HTML),
				func(resp pageResponse) error {
					return renderPageResult(opts, resp.Page, "Published page")
				},
			)
			return pageutil.TranslateMissingScopeError(pageutil.TranslateRateLimitError(err, pageutil.PagesPublishRateLimitMessage))
		},
	}
}

func pushArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return cmdutil.UsageErrorf(cmd, "missing page slug")
	}
	if len(args) < 2 {
		return cmdutil.UsageErrorf(cmd, "missing HTML path (use - for stdin)")
	}
	if len(args) > 2 {
		return cmdutil.UsageErrorf(cmd, "unexpected argument: %s", args[2])
	}
	return nil
}
