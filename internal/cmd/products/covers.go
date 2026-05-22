package products

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/spf13/cobra"
)

func newCoversCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "covers",
		Short: "Manage product cover images",
		Example: `  gumroad products covers add <product_id> --image ./cover.jpg
  gumroad products covers add <product_id> --url https://www.youtube.com/watch?v=qKebcV1jv3A
  gumroad products covers remove <product_id> <cover_id>
  gumroad products covers reorder <product_id> <cover_id> <cover_id>`,
	}

	cmd.AddCommand(newCoversAddCmd())
	cmd.AddCommand(newCoversRemoveCmd())
	cmd.AddCommand(newCoversReorderCmd())
	return cmd
}

func newCoversAddCmd() *cobra.Command {
	var imagePath, coverURL string

	cmd := &cobra.Command{
		Use:   "add <product_id>",
		Short: "Add a cover to a product",
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
			productID := args[0]
			path := productMediaAttachPath(productID, productMediaCover)

			if flags.Changed("url") {
				if err := cmdutil.RequireHTTPURLFlag(c, "url", coverURL); err != nil {
					return err
				}
				params := url.Values{}
				params.Set("url", coverURL)
				return cmdutil.RunRequestWithSuccess(opts, "Adding cover...", http.MethodPost, path, params, productID, "Cover added to product "+productID+".")
			}

			media, err := describeProductMedia([]requestedProductMedia{{Kind: productMediaCover, Path: imagePath}})
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
			return cmdutil.PrintMutationSuccess(opts, data, productID, "Cover added to product "+productID+".")
		},
	}
	cmd.Flags().StringVar(&imagePath, "image", "", "Local JPEG, PNG, or GIF image to upload as a cover")
	cmd.Flags().StringVar(&coverURL, "url", "", "Remote http(s) URL to add as a cover")
	return cmd
}

func newCoversRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <product_id> <cover_id>",
		Short: "Remove a product cover",
		Args:  cmdutil.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID, coverID := args[0], args[1]
			ok, err := cmdutil.ConfirmAction(opts, "Remove cover "+coverID+" from product "+productID+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "remove cover "+coverID, coverID)
			}
			path := cmdutil.JoinPath("products", productID, "covers", coverID)
			return cmdutil.RunRequestWithSuccess(opts, "Removing cover...", http.MethodDelete, path, nil, coverID, "Cover "+coverID+" removed.")
		},
	}
	return cmd
}

func newCoversReorderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reorder <product_id> <cover_id>...",
		Short: "Reorder product covers",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return cmdutil.UsageErrorf(cmd, "provide a product ID and at least one cover ID")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID := args[0]
			params := url.Values{}
			for _, coverID := range args[1:] {
				params.Add("cover_ids[]", coverID)
			}
			path := cmdutil.JoinPath("products", productID)
			return cmdutil.RunRequestWithSuccess(opts, "Reordering covers...", http.MethodPut, path, params, productID, "Covers reordered for product "+productID+".")
		},
	}
	return cmd
}
