package products

import (
	"encoding/json"
	"fmt"
	"net/http"
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
  gumroad products thumbnail remove <product_id>`,
	}

	cmd.AddCommand(newThumbnailSetCmd())
	cmd.AddCommand(newThumbnailRemoveCmd())
	return cmd
}

func newThumbnailSetCmd() *cobra.Command {
	var imagePath string

	cmd := &cobra.Command{
		Use:   "set <product_id>",
		Short: "Set a product thumbnail",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if !c.Flags().Changed("image") {
				return cmdutil.MissingFlagError(c, "--image")
			}
			if strings.TrimSpace(imagePath) == "" {
				return cmdutil.UsageErrorf(c, "--image cannot be empty")
			}
			productID := args[0]
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
			data, err := json.Marshal(map[string]any{"media": results})
			if err != nil {
				return fmt.Errorf("could not encode response: %w", err)
			}
			return cmdutil.PrintMutationSuccess(opts, data, productID, "Thumbnail set for product "+productID+".")
		},
	}
	cmd.Flags().StringVar(&imagePath, "image", "", "Local JPEG, PNG, or GIF image to upload as the thumbnail")
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
