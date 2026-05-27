package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history <product_id>",
		Short: "List local product page snapshots",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return pageutil.History(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]))
		},
	}
}
