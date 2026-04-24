package affiliates

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func NewAffiliatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "affiliates",
		Short: "Manage affiliates",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())

	return cmdutil.PropagateExamples(cmd)
}
