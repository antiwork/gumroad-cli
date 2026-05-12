package comments

import "github.com/spf13/cobra"

func NewCommentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Read and add admin comments on users",
		Example: `  gumroad admin users comments list --user-id 2245593582708
  gumroad admin users comments list --email user@example.com --type Note --type Refund_note --limit 50
  gumroad admin users comments add --user-id 2245593582708 --content "VAT exempt confirmed"`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newAddCmd())

	return cmd
}
