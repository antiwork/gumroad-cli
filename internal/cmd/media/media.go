// Package media implements the `gumroad media` command family: the seller's
// public media library, whose files are hosted on Gumroad's public storage so
// they render on custom/profile pages. Files from `gumroad files upload` live
// in a private bucket and can never be displayed (or moderated) on those
// pages — this family is the page-legal upload path.
package media

import "github.com/spf13/cobra"

// NewMediaCmd returns the root command for public media library operations.
func NewMediaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "media",
		Short: "Upload and manage public media library images",
		Long: "Upload and manage images in your public media library.\n\n" +
			"Media library files are hosted on Gumroad's public CDN, which is the " +
			"only host custom product landing pages and profile pages can display " +
			"images from. Use `media upload` to host an image and embed the " +
			"returned URL in page HTML (see `gumroad pages push`). Files from " +
			"`gumroad files upload` are stored privately and will fail page " +
			"moderation — they cannot be used on pages.",
		Example: `  gumroad media upload ./logo.png
  gumroad media upload ./logo.png --name "Store logo"
  gumroad media list
  gumroad media delete G_abc123`,
	}

	cmd.AddCommand(newUploadCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newDeleteCmd())
	return cmd
}
