package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPagePushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <product_id> [path]",
		Short: "Push a custom HTML page to a product",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			path := pageutil.DefaultPath
			if len(args) == 2 {
				path = args[1]
			}
			return pageutil.Push(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]), path)
		},
	}
}
