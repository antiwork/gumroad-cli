package products

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUnpublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpublish <id>",
		Short: "Unpublish a product",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestWithSuccess(opts, "Unpublishing product...", "PUT", cmdutil.JoinPath("products", args[0], "disable"), url.Values{}, "Product "+args[0]+" unpublished.")
		},
	}
}
