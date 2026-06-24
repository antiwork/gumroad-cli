package user

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page",
		Short: "Manage your profile landing page",
		Example: `  gumroad user page preview ./landing.html
  gumroad user page publish ./landing.html
  gumroad user page publish - < landing.html
  gumroad user page clear --yes
  gumroad user page url`,
	}

	cmd.AddCommand(newPagePreviewCmd())
	cmd.AddCommand(newPagePublishCmd())
	cmd.AddCommand(newPageClearCmd())
	cmd.AddCommand(newPageURLCmd())
	return cmd
}

func profilePageHTMLArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return cmdutil.UsageErrorf(cmd, "unexpected argument: %s", args[1])
	}
	return nil
}

func profilePageHTMLPath(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return pageutil.DefaultHTMLPath
}
