package purchases

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type resendReceiptResponse struct {
	Message string `json:"message"`
}

func newResendReceiptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resend-receipt <purchase-id>",
		Short: "Resend the receipt email for a single purchase",
		Long: `Resend the receipt email for a single purchase to the buyer that purchase
was originally charged to. The buyer email is taken from the purchase
record on the server, not from a flag.`,
		Example: `  gumroad admin purchases resend-receipt 12345
  gumroad admin purchases resend-receipt 12345 --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath("purchases", args[0], "resend_receipt")

			ok, err := cmdutil.ConfirmAction(opts, "Resend the receipt email for purchase "+args[0]+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "resend receipt for purchase "+args[0], args[0])
			}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), url.Values{})
			}

			data, err := admincmd.FetchPostJSON(opts, "Resending receipt...", path, struct{}{})
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[resendReceiptResponse](data)
			if err != nil {
				return err
			}
			return renderResendReceipt(opts, args[0], decoded)
		},
	}

	return cmd
}

func renderResendReceipt(opts cmdutil.Options, purchaseID string, resp resendReceiptResponse) error {
	message := fallback(resp.Message, "Resent receipt for purchase "+purchaseID)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, purchaseID}})
	}

	if opts.Quiet {
		return nil
	}

	return output.Writeln(opts.Out(), opts.Style().Green(message))
}
