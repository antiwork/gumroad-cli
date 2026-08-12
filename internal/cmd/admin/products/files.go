package products

import "github.com/spf13/cobra"

func newFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Work with a product's files",
		Example: `  gumroad admin products files download abc123 f_1
  gumroad admin products files download abc123 f_1 -o review.zip`,
	}

	cmd.AddCommand(newFilesDownloadCmd())

	return cmd
}
