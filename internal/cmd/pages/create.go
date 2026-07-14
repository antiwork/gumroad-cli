package pages

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var title string
	var slug string

	cmd := &cobra.Command{
		Use:   "create [path]",
		Short: "Create a storefront page",
		Long:  "Create a storefront page. Without a path the page starts empty (edit it in the dashboard); with an HTML file (or `-` for stdin) it is created as a custom HTML page.",
		Args:  pageHTMLArgs,
		Example: `  gumroad pages create --title "About"
  gumroad pages create --title "About" --slug about
  gumroad pages create --title "About" ./about.html
  gumroad pages create --title "About" - < about.html`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if title == "" {
				return cmdutil.MissingFlagError(c, "--title")
			}

			params := url.Values{}
			params.Set("title", title)
			if slug != "" {
				params.Set("slug", slug)
			}
			// A path means the page is agent-built custom HTML from the start.
			// Read it before the request so a typo'd path never hits the API.
			if len(args) > 0 {
				input, err := pageutil.ReadHTML(opts.In(), args[0])
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err)
				}
				params.Set("custom_html", input.HTML)
			}

			err := cmdutil.RunRequestDecoded[pageResponse](opts, "Creating page...", http.MethodPost, pagesPath, params, func(resp pageResponse) error {
				return renderPageResult(opts, resp.Page, "Created page")
			})
			return pageutil.TranslateRateLimitError(err, pageutil.PagesPublishRateLimitMessage)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Page title (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "URL slug (defaults to a slug generated from the title)")

	return cmd
}

func pageHTMLArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return cmdutil.UsageErrorf(cmd, "unexpected argument: %s", args[1])
	}
	return nil
}

func renderPageResult(opts cmdutil.Options, p page, action string) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{p.Slug, p.Title, pageKind(p), pageURL(p)}})
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold(action+" "+p.Title)); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Slug: %s (%s)\n", p.Slug, pageKind(p)); err != nil {
		return err
	}
	if url := pageURL(p); url != "" {
		return output.Writef(opts.Out(), "Live at %s\n", url)
	}
	return nil
}
