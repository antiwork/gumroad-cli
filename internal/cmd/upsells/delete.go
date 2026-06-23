package upsells

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <upsell_id>",
		Short: "Delete an upsell or cross-sell",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			id := args[0]

			ok, err := cmdutil.ConfirmAction(opts, "Delete upsell "+id+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "delete upsell "+id, id)
			}

			return cmdutil.RunRequestWithSuccess(opts, "Deleting upsell...", http.MethodDelete, cmdutil.JoinPath("upsells", id), url.Values{}, id, "Upsell "+id+" deleted.")
		},
	}
}
