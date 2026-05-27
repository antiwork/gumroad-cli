package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear <product_id>",
		Short: "Clear a product custom HTML page",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return pageutil.Clear(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]))
		},
	}
}
