package admin

import "github.com/spf13/cobra"

func NewAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "admin",
		Short:   "Run Gumroad admin commands",
		Long:    "Run internal Gumroad admin API commands with a separate admin token.",
		Example: "  gumroad admin --help",
	}

	return cmd
}
