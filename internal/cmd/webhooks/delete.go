package webhooks

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a webhook subscription",
		Long: `Delete a webhook subscription.

Note: This only succeeds when the token's OAuth app matches the subscription's app.`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			ok, err := cmdutil.ConfirmAction(opts, "Delete webhook "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete webhook "+args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Deleting webhook...", "DELETE", cmdutil.JoinPath("resource_subscriptions", args[0]), url.Values{}, "Webhook "+args[0]+" deleted.")
		},
	}
}
