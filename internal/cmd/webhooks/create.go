package webhooks

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var resource, postURL string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook subscription",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if resource == "" {
				return cmdutil.MissingFlagError(c, "--resource")
			}
			if postURL == "" {
				return cmdutil.MissingFlagError(c, "--url")
			}
			if err := cmdutil.RequireHTTPURLFlag(c, "url", postURL); err != nil {
				return err
			}

			params := url.Values{}
			params.Set("resource_name", resource)
			params.Set("post_url", postURL)

			return cmdutil.RunRequestWithSuccess(opts, "Creating webhook...", "PUT", "/resource_subscriptions", params, "Webhook created for "+resource+".")
		},
	}

	cmd.Flags().StringVar(&resource, "resource", "", "Resource name (required)")
	cmd.Flags().StringVar(&postURL, "url", "", "POST URL for the webhook (required)")

	return cmd
}
