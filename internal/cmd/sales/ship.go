package sales

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newShipCmd() *cobra.Command {
	var trackingURL string

	cmd := &cobra.Command{
		Use:   "ship <id>",
		Short: "Mark a sale as shipped",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireHTTPURLFlag(c, "tracking-url", trackingURL); err != nil {
				return err
			}

			params := url.Values{}
			if trackingURL != "" {
				params.Set("tracking_url", trackingURL)
			}

			return cmdutil.RunRequestWithSuccess(opts, "Marking as shipped...", "PUT", cmdutil.JoinPath("sales", args[0], "mark_as_shipped"), params, "Sale "+args[0]+" marked as shipped.")
		},
	}

	cmd.Flags().StringVar(&trackingURL, "tracking-url", "", "Tracking URL")

	return cmd
}
