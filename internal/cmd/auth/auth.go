package auth

import (
	"github.com/spf13/cobra"
)

func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Example: "  gumroad auth login\n" +
			"  gumroad auth status\n" +
			"  gumroad auth token\n" +
			"  gumroad auth logout",
	}

	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newTokenCmd())
	cmd.AddCommand(newLogoutCmd())

	return cmd
}
