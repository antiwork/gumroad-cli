package products

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func validateProductCategoryFlags(cmd *cobra.Command) error {
	flags := cmd.Flags()
	if flags.Changed("category") && flags.Changed("taxonomy-id") {
		return cmdutil.UsageErrorf(cmd, "specify either --category or --taxonomy-id, not both")
	}
	return nil
}
