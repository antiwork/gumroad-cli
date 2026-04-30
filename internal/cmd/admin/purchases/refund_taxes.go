package purchases

import (
	"errors"
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

type refundTaxesRequest struct {
	Email         string `json:"email"`
	Note          string `json:"note,omitempty"`
	BusinessVATID string `json:"business_vat_id,omitempty"`
}

type refundTaxesResponse struct {
	Message  string   `json:"message"`
	Purchase purchase `json:"purchase"`
}

func newRefundTaxesCmd() *cobra.Command {
	var (
		email         string
		note          string
		businessVATID string
	)

	cmd := &cobra.Command{
		Use:   "refund-taxes <purchase-id>",
		Short: "Refund only the Gumroad-collected taxes on a purchase",
		Long: `Refund the Gumroad-collected taxes on a purchase without touching the
purchase price. The buyer email is required as a sanity check against
the purchase record.

--note records an admin-side note alongside the tax refund. --business-vat-id
attaches a buyer-supplied VAT ID to the refund record (commonly required
when a B2B buyer needs the tax reversed because they self-account for VAT).`,
		Example: `  gumroad admin purchases refund-taxes 12345 --email buyer@example.com
  gumroad admin purchases refund-taxes 12345 --email buyer@example.com --business-vat-id GB123456789
  gumroad admin purchases refund-taxes 12345 --email buyer@example.com --note "buyer self-accounts for VAT"`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			path := cmdutil.JoinPath("purchases", args[0], "refund_taxes")

			ok, err := cmdutil.ConfirmAction(opts, "Refund taxes on purchase "+args[0]+"? This is irreversible.")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "refund taxes on purchase "+args[0], args[0])
			}

			req := refundTaxesRequest{
				Email:         email,
				Note:          note,
				BusinessVATID: businessVATID,
			}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), refundTaxesDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Refunding taxes...", path, req)
			if err != nil {
				return wrapRefundTaxesError(args[0], err)
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[refundTaxesResponse](data)
			if err != nil {
				return err
			}
			return renderRefundTaxes(opts, args[0], decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")
	cmd.Flags().StringVar(&note, "note", "", "Admin note attached to the tax refund")
	cmd.Flags().StringVar(&businessVATID, "business-vat-id", "", "Buyer's business VAT ID")

	return cmd
}

func wrapRefundTaxesError(purchaseID string, err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message: fmt.Sprintf(
				"refund-taxes request failed: %s. Verify status with 'gumroad admin purchases view %s' before retrying to avoid duplicate tax refunds",
				apiErr.Message, purchaseID,
			),
			Hint: apiErr.Hint,
		}
	}
	return fmt.Errorf("refund-taxes request failed: %w. Verify status with 'gumroad admin purchases view %s' before retrying to avoid duplicate tax refunds", err, purchaseID)
}

func refundTaxesDryRunParams(req refundTaxesRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	if req.Note != "" {
		params.Set("note", req.Note)
	}
	if req.BusinessVATID != "" {
		params.Set("business_vat_id", req.BusinessVATID)
	}
	return params
}

func renderRefundTaxes(opts cmdutil.Options, purchaseID string, resp refundTaxesResponse) error {
	message := fallback(resp.Message, "Refunded taxes for purchase "+purchaseID)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{"true", message, fallback(resp.Purchase.ID, purchaseID)},
		})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	if resp.Purchase.ID != "" {
		return renderPurchase(opts, resp.Purchase)
	}
	return nil
}
