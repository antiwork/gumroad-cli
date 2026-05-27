package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <product_id>",
		Short: "Print a product page URL",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return pageutil.URL(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]))
		},
	}
}
