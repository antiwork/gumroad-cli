package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageRestoreCmd() *cobra.Command {
	var snapshot int

	cmd := &cobra.Command{
		Use:   "restore <product_id>",
		Short: "Restore a product page from a local snapshot",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return pageutil.Restore(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]), snapshot)
		},
	}
	cmd.Flags().IntVar(&snapshot, "snapshot", 1, "Snapshot index from page history")
	return cmd
}
