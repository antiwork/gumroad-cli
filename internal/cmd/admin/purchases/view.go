package purchases

import (
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type purchaseResponse struct {
	Purchase purchase `json:"purchase"`
}

type purchase struct {
	ID                              string      `json:"id"`
	Email                           string      `json:"email"`
	SellerEmail                     string      `json:"seller_email"`
	ProductName                     string      `json:"product_name"`
	ProductAlias                    string      `json:"link_name"`
	ProductID                       string      `json:"product_id"`
	FormattedTotalPrice             string      `json:"formatted_total_price"`
	PriceCents                      api.JSONInt `json:"price_cents"`
	CurrencyType                    string      `json:"currency_type"`
	AmountRefundableCentsInCurrency api.JSONInt `json:"amount_refundable_cents_in_currency"`
	PurchaseState                   string      `json:"purchase_state"`
	RefundStatus                    string      `json:"refund_status"`
	CreatedAt                       string      `json:"created_at"`
	ReceiptURL                      string      `json:"receipt_url"`
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <purchase-id>",
		Short: "View an admin purchase record",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath("purchases", args[0])
			return admincmd.RunGetDecoded[purchaseResponse](opts, "Fetching purchase...", path, url.Values{}, func(resp purchaseResponse) error {
				return renderPurchase(opts, resp.Purchase)
			})
		},
	}
}

func productLabel(p purchase) string {
	if p.ProductName != "" {
		return p.ProductName
	}
	if p.ProductAlias != "" {
		return p.ProductAlias
	}
	return p.ProductID
}

func renderPurchase(opts cmdutil.Options, p purchase) error {
	product := productLabel(p)

	amount := p.FormattedTotalPrice
	if amount == "" && p.PriceCents != 0 {
		amount = fmt.Sprintf("%d cents", p.PriceCents)
	}

	status := p.PurchaseState
	if p.RefundStatus != "" {
		if status != "" {
			status += ", "
		}
		status += p.RefundStatus
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{p.ID, p.Email, p.SellerEmail, product, amount, status, p.CreatedAt, p.ReceiptURL},
		})
	}

	style := opts.Style()
	headlineFromID := false
	headline := product
	if headline == "" {
		headline = p.ID
		headlineFromID = true
	}
	if amount != "" {
		headline += "  " + amount
	}
	if err := output.Writeln(opts.Out(), style.Bold(headline)); err != nil {
		return err
	}
	if !headlineFromID {
		if err := output.Writef(opts.Out(), "Purchase ID: %s\n", p.ID); err != nil {
			return err
		}
	}
	if p.Email != "" {
		if err := output.Writef(opts.Out(), "Buyer: %s\n", p.Email); err != nil {
			return err
		}
	}
	if p.SellerEmail != "" {
		if err := output.Writef(opts.Out(), "Seller: %s\n", p.SellerEmail); err != nil {
			return err
		}
	}
	if status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", status); err != nil {
			return err
		}
	}
	if p.CreatedAt != "" {
		if err := output.Writef(opts.Out(), "Date: %s\n", p.CreatedAt); err != nil {
			return err
		}
	}
	if p.ReceiptURL != "" {
		return output.Writef(opts.Out(), "Receipt: %s\n", p.ReceiptURL)
	}
	return nil
}
