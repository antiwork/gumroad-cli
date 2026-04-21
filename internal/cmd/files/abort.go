package files

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newAbortCmd() *cobra.Command {
	var uploadID, key string
	c := &cobra.Command{
		Use:   "abort",
		Short: "Abort an orphaned multipart upload on S3",
		Args:  cmdutil.ExactArgs(0),
		Long: "Abort an orphaned multipart upload using the upload_id and key returned in a " +
			"failed upload's recovery fields. Use this when a previous `gumroad files upload` " +
			"left a multipart upload behind (cleanup_failed error, or a state-unknown finalize " +
			"that you decided not to retry).",
		Example: `  gumroad files abort --upload-id up-123 --key attachments/u/k/original/pack.zip
  gumroad files abort --upload-id "$(jq -r .error.recovery.upload_id < err.json)" \
    --key "$(jq -r .error.recovery.key < err.json)"`,
		RunE: func(c *cobra.Command, _ []string) error {
			opts := cmdutil.OptionsFrom(c)
			if uploadID == "" {
				return cmdutil.MissingFlagError(c, "--upload-id")
			}
			if key == "" {
				return cmdutil.MissingFlagError(c, "--key")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Abort multipart upload "+uploadID+"? This is irreversible.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "abort multipart upload "+uploadID, uploadID)
			}

			params := url.Values{}
			params.Set("upload_id", uploadID)
			params.Set("key", key)
			return cmdutil.RunRequestWithSuccess(opts, "Aborting multipart upload...", "POST", "/files/abort", params, uploadID, "Multipart upload aborted.")
		},
	}
	c.Flags().StringVar(&uploadID, "upload-id", "", "S3 multipart upload_id to abort (required)")
	c.Flags().StringVar(&key, "key", "", "S3 object key for the upload (required)")
	return c
}
