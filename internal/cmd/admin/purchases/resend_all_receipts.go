package purchases

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type resendAllReceiptsRequest struct {
	Email string `json:"email"`
}

type resendAllReceiptsResponse struct {
	Message string      `json:"message"`
	Count   api.JSONInt `json:"count"`
}

func newResendAllReceiptsCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "resend-all-receipts",
		Short: "Resend grouped receipts for every successful purchase by a buyer",
		Long: `Resend a single grouped receipt covering every successful purchase
made by the given email address. Useful when a buyer reports never
receiving any of their receipts.`,
		Example: `  gumroad admin purchases resend-all-receipts --email buyer@example.com
  gumroad admin purchases resend-all-receipts --email buyer@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Resend grouped receipts for every successful purchase by "+email+"?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "resend all receipts for "+email, email)
			}

			req := resendAllReceiptsRequest{Email: email}

			if opts.DryRun {
				params := url.Values{}
				params.Set("email", email)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath("/purchases/resend_all_receipts"), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Resending all receipts...", "/purchases/resend_all_receipts", req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[resendAllReceiptsResponse](data)
			if err != nil {
				return err
			}
			return renderResendAllReceipts(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")

	return cmd
}

func renderResendAllReceipts(opts cmdutil.Options, email string, resp resendAllReceiptsResponse) error {
	message := fallback(resp.Message, fmt.Sprintf("Resent grouped receipts to %s", email))
	count := fmt.Sprintf("%d", resp.Count)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, email, count}})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	if resp.Count > 0 {
		return output.Writef(opts.Out(), "Purchases included: %d\n", resp.Count)
	}
	return nil
}
