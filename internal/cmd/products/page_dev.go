package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageDevCmd() *cobra.Command {
	var port int
	var shouldOpen bool

	cmd := &cobra.Command{
		Use:   "dev <product_id> [path]",
		Short: "Run a local page preview server",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			path := pageutil.DefaultPath
			if len(args) == 2 {
				path = args[1]
			}
			return pageutil.Dev(cmdutil.OptionsFrom(c), pageutil.ProductTarget(args[0]), path, port, shouldOpen)
		},
	}
	cmd.Flags().IntVar(&port, "port", pageutil.DefaultDevPort, "Local dev server port")
	cmd.Flags().BoolVar(&shouldOpen, "open", false, "Open the dev server in the browser")
	return cmd
}
