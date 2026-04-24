package licenses

import "github.com/spf13/cobra"

func NewLicensesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "licenses",
		Short: "Read admin license records",
		Example: `  echo "$LICENSE_KEY" | gumroad admin licenses lookup
  gumroad admin licenses lookup --key <license-key> --json`,
	}

	cmd.AddCommand(newLookupCmd())

	return cmd
}
