package products

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/spf13/cobra"
)

func newThumbnailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thumbnail",
		Short: "Manage a product thumbnail",
		Example: `  gumroad products thumbnail set <product_id> --image ./thumb.jpg
  gumroad products thumbnail set <product_id> --url https://example.com/thumb.png
  gumroad products thumbnail remove <product_id>`,
	}

	cmd.AddCommand(newThumbnailSetCmd())
	cmd.AddCommand(newThumbnailRemoveCmd())
	return cmd
}

func newThumbnailSetCmd() *cobra.Command {
	var imagePath, thumbnailURL string

	cmd := &cobra.Command{
		Use:   "set <product_id>",
		Short: "Set a product thumbnail",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			flags := c.Flags()
			if !flags.Changed("image") && !flags.Changed("url") {
				return cmdutil.UsageErrorf(c, "provide --image or --url")
			}
			if flags.Changed("image") && flags.Changed("url") {
				return cmdutil.UsageErrorf(c, "--image and --url cannot be used together")
			}
			if flags.Changed("image") && strings.TrimSpace(imagePath) == "" {
				return cmdutil.UsageErrorf(c, "--image cannot be empty")
			}
			productID := args[0]
			path := productMediaAttachPath(productID, productMediaThumbnail)

			if flags.Changed("url") {
				if err := cmdutil.RequireHTTPURLFlag(c, "url", thumbnailURL); err != nil {
					return err
				}
				params := url.Values{}
				params.Set("url", thumbnailURL)
				return cmdutil.RunRequestWithSuccess(opts, "Setting thumbnail...", http.MethodPost, path, params, productID, "Thumbnail set for product "+productID+".")
			}

			media, err := describeProductMedia([]requestedProductMedia{{Kind: productMediaThumbnail, Path: imagePath}})
			if err != nil {
				return err
			}
			if opts.DryRun {
				return renderStandaloneProductMediaDryRun(opts, media, productMediaDryRunRequests(productID, media))
			}

			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)
			results, err := uploadAndAttachProductMedia(opts, client, productID, media, "")
			if err != nil {
				return err
			}
			data, err := productMediaSingleAttachResult(results)
			if err != nil {
				return err
			}
			return cmdutil.PrintMutationSuccess(opts, data, productID, "Thumbnail set for product "+productID+".")
		},
	}
	cmd.Flags().StringVar(&imagePath, "image", "", "Local JPEG, PNG, or GIF image to upload as the thumbnail")
	cmd.Flags().StringVar(&thumbnailURL, "url", "", "Remote http(s) image URL to set as the thumbnail")
	return cmd
}

func newThumbnailRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <product_id>",
		Short: "Remove a product thumbnail",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID := args[0]
			ok, err := cmdutil.ConfirmAction(opts, "Remove thumbnail from product "+productID+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "remove thumbnail from product "+productID, productID)
			}
			path := productMediaAttachPath(productID, productMediaThumbnail)
			return cmdutil.RunRequestWithSuccess(opts, "Removing thumbnail...", http.MethodDelete, path, nil, productID, "Thumbnail removed from product "+productID+".")
		},
	}
	return cmd
}
