package purchases

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type purchaseResponse struct {
	Purchase purchase `json:"purchase"`
}

type Purchase = purchase

type purchase struct {
	ID                              string        `json:"id"`
	Email                           string        `json:"email"`
	SellerEmail                     string        `json:"seller_email"`
	Seller                          seller        `json:"seller"`
	ProductName                     string        `json:"product_name"`
	ProductAlias                    string        `json:"link_name"`
	ProductID                       string        `json:"product_id"`
	FormattedTotalPrice             string        `json:"formatted_total_price"`
	PriceCents                      api.JSONInt   `json:"price_cents"`
	CurrencyType                    string        `json:"currency_type"`
	AmountRefundableCentsInCurrency api.JSONInt   `json:"amount_refundable_cents_in_currency"`
	PurchaseState                   string        `json:"purchase_state"`
	RefundStatus                    string        `json:"refund_status"`
	CreatedAt                       string        `json:"created_at"`
	ReceiptURL                      string        `json:"receipt_url"`
	ChargebackDate                  string        `json:"chargeback_date"`
	CountryMismatches               mismatches    `json:"country_mismatches"`
	EarlyFraudWarning               *fraudWarning `json:"early_fraud_warning"`
}

type seller struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type mismatches struct {
	BillingVsIP   bool `json:"billing_vs_ip"`
	BillingVsCard bool `json:"billing_vs_card"`
	IPVsCard      bool `json:"ip_vs_card"`
}

type fraudWarning struct {
	ID string `json:"id"`
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

func ProductLabel(p Purchase) string {
	return productLabel(p)
}

func amountLabel(p purchase) string {
	if p.FormattedTotalPrice != "" {
		return p.FormattedTotalPrice
	}
	if p.PriceCents != 0 {
		return fmt.Sprintf("%d cents", p.PriceCents)
	}
	return ""
}

func AmountLabel(p Purchase) string {
	return amountLabel(p)
}

func statusLabel(p purchase) string {
	status := p.PurchaseState
	if p.RefundStatus != "" {
		if status != "" {
			status += ", "
		}
		status += p.RefundStatus
	}
	return status
}

func StatusLabel(p Purchase) string {
	return statusLabel(p)
}

func SellerLabel(p Purchase) string {
	switch {
	case p.Seller.Email != "":
		return p.Seller.Email
	case p.SellerEmail != "":
		return p.SellerEmail
	case p.Seller.Name != "":
		return p.Seller.Name
	default:
		return p.Seller.ID
	}
}

func RiskFlagsLabel(p Purchase) string {
	flags := make([]string, 0, 3)
	if p.ChargebackDate != "" {
		flags = append(flags, "CB")
	}
	if p.EarlyFraudWarning != nil {
		flags = append(flags, "EFW")
	}
	if p.CountryMismatches.BillingVsIP || p.CountryMismatches.BillingVsCard || p.CountryMismatches.IPVsCard {
		flags = append(flags, "CM")
	}
	return strings.Join(flags, ",")
}

func renderPurchase(opts cmdutil.Options, p purchase) error {
	product := productLabel(p)
	amount := amountLabel(p)
	status := statusLabel(p)

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
