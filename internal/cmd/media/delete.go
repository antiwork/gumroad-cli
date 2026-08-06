package media

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
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
			if err := output.ValidateJQExpression(opts.JQExpr); err != nil {
				return err
			}
			ok, err := cmdutil.ConfirmAction(opts, "Delete media file "+args[0]+"? Pages embedding it will show a broken image.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete media file "+args[0], args[0])
			}
			path := cmdutil.JoinPath("media", args[0])
			params := url.Values{}
			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodDelete, path, params)
			}
			token, err := config.Token()
			if err != nil {
				return err
			}
			data, err := cmdutil.RunRequestWithTokenData(opts, token, "Deleting media file...", http.MethodDelete, path, params)
			if err != nil {
				return mediaDeleteRequestError(err, args[0])
			}
			if err := cmdutil.PrintMutationSuccess(opts, data, args[0], "Media file "+args[0]+" deleted."); err != nil {
				return mediaDeleteOutputError(err, args[0])
			}
			return nil

		},
	}
}
