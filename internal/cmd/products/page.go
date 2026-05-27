package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page",
		Short: "Manage a product custom HTML page",
		Example: `  gumroad products page push <product_id> ./landing.html
  gumroad products page preview <product_id> ./landing.html
  gumroad products page dev <product_id> ./landing.html --open
  gumroad products page clear <product_id>
  gumroad products page restore <product_id>
  gumroad products page history <product_id>
  gumroad products page url <product_id>`,
	}

	cmd.AddCommand(newPagePushCmd())
	cmd.AddCommand(newPagePreviewCmd())
	cmd.AddCommand(newPageDevCmd())
	cmd.AddCommand(newPageClearCmd())
	cmd.AddCommand(newPageRestoreCmd())
	cmd.AddCommand(newPageHistoryCmd())
	cmd.AddCommand(newPageURLCmd())
	cmdutil.PropagateExamples(cmd)
	return cmd
}
