package sales

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newResendReceiptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resend-receipt <id>",
		Short: "Resend a receipt",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestWithSuccess(opts, "Resending receipt...", "POST", cmdutil.JoinPath("sales", args[0], "resend_receipt"), url.Values{}, args[0], "Receipt resent for sale "+args[0]+".")
		},
	}
}
