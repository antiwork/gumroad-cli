package emails

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type sendEmailResponse struct {
	Email emailRecord `json:"email"`
}

func newSendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send <id>",
		Short: "Send an audience email",
		Long:  "Publish and send an audience email to its recipients. This action is irreversible.",
		Example: `  gumroad emails send <id> --yes
  gumroad emails send <id> --json --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			ok, err := cmdutil.ConfirmAction(opts, "Send email "+args[0]+" to its audience now?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "send email "+args[0], args[0])
			}

			return cmdutil.RunRequestDecoded[sendEmailResponse](opts, "Sending email...", "POST", cmdutil.JoinPath("emails", args[0], "send"), url.Values{}, func(resp sendEmailResponse) error {
				item := resp.Email
				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{{item.ID, item.Subject, item.State}})
				}
				if opts.Quiet {
					return nil
				}
				style := opts.Style()
				return output.Writef(opts.Out(), "%s %s (%s) [%s]\n",
					style.Bold("Sent email:"), item.Subject, style.Dim(item.ID), item.State)
			})
		},
	}
}
