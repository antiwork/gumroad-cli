package pages

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// The special slug the pages dashboard uses for the seller's profile landing
// page. Pushing to it goes through the profile custom HTML endpoints instead
// of the slugged /pages API — the profile root page is not addressable there.
const profileSlug = "profile"

const pagesPath = "/pages"

type page struct {
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	Content    *string `json:"content"`
	CustomHTML *string `json:"custom_html"`
	URL        *string `json:"url"`
}

type pageResponse struct {
	Success bool `json:"success"`
	Page    page `json:"page"`
}

func NewPagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pages",
		Short: "Manage your storefront pages",
		Long:  "Manage your storefront pages: the slugged pages that serve under your store at /<slug>.\n\nThe special slug \"profile\" is your profile landing page (your store's home page); `pages push profile` replaces it with custom HTML the same way `user page publish` does.",
		Example: `  gumroad pages list
  gumroad pages create --title "About" --slug about
  gumroad pages create --title "About" ./about.html
  gumroad pages pull about
  gumroad pages scaffold about
  gumroad pages push about ./about.html
  gumroad pages push profile ./landing.html
  gumroad pages preview ./about.html`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newPullCmd())
	cmd.AddCommand(newScaffoldCmd())
	cmd.AddCommand(newPushCmd())
	cmd.AddCommand(newPreviewCmd())
	return cmd
}

func pagePath(slug string) string {
	return cmdutil.JoinPath("pages", slug)
}

// A page's kind for list output: what the seller would edit it with.
func pageKind(p page) string {
	if p.CustomHTML != nil && *p.CustomHTML != "" {
		return "custom HTML"
	}
	return "rich text"
}

func pageURL(p page) string {
	if p.URL == nil {
		return ""
	}
	return *p.URL
}
