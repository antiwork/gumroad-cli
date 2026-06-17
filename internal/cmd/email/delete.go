package email

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an audience email",
		Long:  "Delete a draft or scheduled audience email.",
		Example: `  gumroad email delete <id> --yes
  gumroad email delete <id> --json --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			ok, err := cmdutil.ConfirmAction(opts, "Delete email "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete email "+args[0], args[0])
			}

			return cmdutil.RunRequestWithResource(opts, "Deleting email...", "DELETE", cmdutil.JoinPath("installments", args[0]), url.Values{}, args[0], "Email "+args[0]+" deleted.")
		},
	}
}
