package pages

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

type seoFields struct {
	SeoTitle       *string `json:"seo_title"`
	SeoDescription *string `json:"seo_description"`
}

type seoPageResponse struct {
	Page seoFields `json:"page"`
}

type seoUserResponse struct {
	User seoFields `json:"user"`
}

func newSeoCmd() *cobra.Command {
	var title string
	var description string

	cmd := &cobra.Command{
		Use:   "seo <slug>",
		Short: "Show or set a page's search title and description",
		Long:  "Show or set the title and description that search engines and shared links show for a storefront page. Without flags, print the current values. Pass an empty value to go back to the default.\n\nUse the slug \"profile\" for your store's home page.",
		Args:  cmdutil.ExactArgs(1),
		Example: `  gumroad pages seo about
  gumroad pages seo about --title "About Jane's studio" --description "Hand-lettered posters and prints."
  gumroad pages seo profile --title "Hand-lettered posters"
  gumroad pages seo about --description ""`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			slug := args[0]

			params := url.Values{}
			if c.Flags().Changed("title") {
				params.Set("seo_title", title)
			}
			if c.Flags().Changed("description") {
				params.Set("seo_description", description)
			}

			// The home page's fields live on the seller profile, so they go
			// through the user endpoint, the same as `user update`.
			if slug == profileSlug {
				method, message := http.MethodGet, "Fetching search fields..."
				if len(params) > 0 {
					method, message = http.MethodPut, "Saving search fields..."
				}
				return cmdutil.RunRequestDecoded[seoUserResponse](opts, message, method, "/user", params, func(resp seoUserResponse) error {
					return renderSeo(opts, slug, resp.User, method == http.MethodPut)
				})
			}

			if len(params) == 0 {
				return cmdutil.RunRequestDecoded[seoPageResponse](opts, "Fetching search fields...", http.MethodGet, pagePath(slug), url.Values{"source_only": {"true"}}, func(resp seoPageResponse) error {
					return renderSeo(opts, slug, resp.Page, false)
				})
			}
			err := cmdutil.RunRequestDecoded[seoPageResponse](opts, "Saving search fields...", http.MethodPut, pagePath(slug), params, func(resp seoPageResponse) error {
				return renderSeo(opts, slug, resp.Page, true)
			})
			return pageutil.TranslateMissingScopeError(err)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Search title (empty to use the default)")
	cmd.Flags().StringVar(&description, "description", "", "Search description (empty to use the default)")

	return cmd
}

// --quiet silences the echo after a save; a read still prints what was asked for.
func renderSeo(opts cmdutil.Options, slug string, fields seoFields, saved bool) error {
	title, description := valueOrEmpty(fields.SeoTitle), valueOrEmpty(fields.SeoDescription)
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{slug, title, description}})
	}
	if opts.Quiet && saved {
		return nil
	}

	if err := output.Writef(opts.Out(), "Search title: %s\n", orDefault(title)); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Search description: %s\n", orDefault(description))
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func orDefault(value string) string {
	if value == "" {
		return "(default)"
	}
	return value
}
