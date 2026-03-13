package webhooks

import (
	"github.com/spf13/cobra"
)

func NewWebhooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Manage resource subscriptions (webhooks)",
		Long: `Manage webhook subscriptions for resource events.

Note: delete only succeeds when the token's OAuth app matches the subscription's app.`,
		Example: `  gumroad webhooks list --resource sale
  gumroad webhooks create --resource sale --url https://example.com/hook`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
