// Package files implements the `gumroad files` command family.
package files

import "github.com/spf13/cobra"

// NewFilesCmd returns the root command for file operations.
func NewFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Upload and manage file attachments",
		Long: "Upload and manage files in your Gumroad seller namespace.\n\n" +
			"Uploaded files live under your seller's attachments namespace and " +
			"can be attached to products (see `gumroad products create --help`). " +
			"After a failed `upload` returns complete_state_unknown recovery " +
			"fields, use `gumroad files complete --recovery ...` to finalize " +
			"without re-uploading, or `gumroad files abort` to reclaim the " +
			"orphaned multipart upload.",
		Example: `  gumroad files upload ./pack.zip
  gumroad files upload ./pack.zip --name "Art Pack.zip"
  gumroad files upload ./pack.zip --json --jq '.file_url'
  gumroad files complete --recovery recovery.json
  gumroad files abort --upload-id up-123 --key attachments/u/k/original/pack.zip`,
	}

	cmd.AddCommand(newUploadCmd())
	cmd.AddCommand(newCompleteCmd())
	cmd.AddCommand(newAbortCmd())
	return cmd
}
