package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page",
		Short: "Manage a product landing page",
		Example: `  gumroad products page preview <product_id> ./landing.html
  gumroad products page publish <product_id> ./landing.html
  gumroad products page publish <product_id> - < landing.html
  gumroad products page clear <product_id> --yes
  gumroad products page url <product_id>`,
	}

	cmd.AddCommand(newPagePreviewCmd())
	cmd.AddCommand(newPagePublishCmd())
	cmd.AddCommand(newPageClearCmd())
	cmd.AddCommand(newPageURLCmd())
	return cmd
}

func productPageHTMLArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return cmdutil.UsageErrorf(cmd, "missing required argument: <product_id>")
	}
	if len(args) > 2 {
		return cmdutil.UsageErrorf(cmd, "unexpected argument: %s", args[2])
	}
	return nil
}

func productPageHTMLPath(args []string) string {
	if len(args) > 1 {
		return args[1]
	}
	return pageutil.DefaultHTMLPath
}
