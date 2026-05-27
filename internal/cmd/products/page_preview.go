package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPagePreviewCmd() *cobra.Command {
	var diff bool

	cmd := &cobra.Command{
		Use:   "preview <product_id> [path]",
		Short: "Preview page sanitization without publishing",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			path := pageutil.DefaultPath
			if len(args) == 2 {
				path = args[1]
			}
			return pageutil.Preview(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]), path, diff)
		},
	}
	cmd.Flags().BoolVar(&diff, "diff", false, "Show a unified diff against the live page")
	return cmd
}
