package products

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <id>",
		Short: "Publish a product",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestWithSuccess(opts, "Publishing product...", "PUT", cmdutil.JoinPath("products", args[0], "enable"), url.Values{}, "Product "+args[0]+" published.")
		},
	}
}
