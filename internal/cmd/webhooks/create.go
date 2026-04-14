package webhooks

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type createWebhookResponse struct {
	ResourceSubscription struct {
		ID           string `json:"id"`
		ResourceName string `json:"resource_name"`
		PostURL      string `json:"post_url"`
	} `json:"resource_subscription"`
}

func newCreateCmd() *cobra.Command {
	var resource, postURL string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook",
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

			return cmdutil.RunRequestDecoded[createWebhookResponse](opts,
				"Creating webhook...", "PUT", "/resource_subscriptions", params,
				func(resp createWebhookResponse) error {
					ws := resp.ResourceSubscription
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{ws.ID, ws.ResourceName, ws.PostURL}})
					}
					if opts.Quiet {
						return nil
					}
					s := opts.Style()
					return output.Writef(opts.Out(), "%s %s → %s (%s)\n",
						s.Bold("Created webhook:"), ws.ResourceName, ws.PostURL, s.Dim(ws.ID))
				})
		},
	}

	cmd.Flags().StringVar(&resource, "resource", "", "Resource name (required)")
	cmd.Flags().StringVar(&postURL, "url", "", "POST URL for the webhook (required)")

	return cmd
}
