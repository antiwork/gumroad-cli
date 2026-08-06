package media

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a public media library file",
		Long: `Delete a public media library file.

Any page still embedding the file's URL will show a broken image after
deletion.`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			ok, err := cmdutil.ConfirmAction(opts, "Delete media file "+args[0]+"? Pages embedding it will show a broken image.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete media file "+args[0], args[0])
			}

			return cmdutil.RunRequestWithSuccess(opts, "Deleting media file...", "DELETE", cmdutil.JoinPath("media", args[0]), url.Values{}, args[0], "Media file "+args[0]+" deleted.")
		},
	}
}
