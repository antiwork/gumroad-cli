package purchases

import (
	"fmt"
	"io"
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
	ID                              string                   `json:"id"`
	Email                           string                   `json:"email"`
	SellerEmail                     string                   `json:"seller_email"`
	ProductName                     string                   `json:"product_name"`
	ProductAlias                    string                   `json:"link_name"`
	ProductID                       string                   `json:"product_id"`
	FormattedTotalPrice             string                   `json:"formatted_total_price"`
	PriceCents                      api.JSONInt              `json:"price_cents"`
	CurrencyType                    string                   `json:"currency_type"`
	AmountRefundableCentsInCurrency api.JSONInt              `json:"amount_refundable_cents_in_currency"`
	PurchaseState                   string                   `json:"purchase_state"`
	RefundStatus                    string                   `json:"refund_status"`
	ChargebackDate                  string                   `json:"chargeback_date"`
	CreatedAt                       string                   `json:"created_at"`
	ReceiptURL                      string                   `json:"receipt_url"`
	ChargeProcessor                 string                   `json:"charge_processor"`
	PaypalOrderID                   string                   `json:"paypal_order_id"`
	IPAddress                       string                   `json:"ip_address"`
	IPCountry                       string                   `json:"ip_country"`
	BillingCountry                  string                   `json:"billing_country"`
	CardCountry                     string                   `json:"card_country"`
	CountryMismatches               countryMismatches        `json:"country_mismatches"`
	Card                            purchaseCard             `json:"card"`
	Dispute                         *purchaseDispute         `json:"dispute"`
	EarlyFraudWarning               *earlyFraudWarning       `json:"early_fraud_warning"`
	AffiliateCredit                 *purchaseAffiliateCredit `json:"affiliate_credit"`
	Clusters                        *purchaseClusters        `json:"clusters"`
	Seller                          *purchaseSeller          `json:"seller"`
}

type purchaseSeller struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type countryMismatches struct {
	BillingVsIP   bool `json:"billing_vs_ip"`
	BillingVsCard bool `json:"billing_vs_card"`
	IPVsCard      bool `json:"ip_vs_card"`
}

type purchaseCard struct {
	Bin         string      `json:"bin"`
	Type        string      `json:"type"`
	Visual      string      `json:"visual"`
	ExpiryMonth api.JSONInt `json:"expiry_month"`
	ExpiryYear  api.JSONInt `json:"expiry_year"`
}

type purchaseDispute struct {
	ID                       string `json:"id"`
	State                    string `json:"state"`
	Reason                   string `json:"reason"`
	ChargeProcessorDisputeID string `json:"charge_processor_dispute_id"`
	CreatedAt                string `json:"created_at"`
	InitiatedAt              string `json:"initiated_at"`
	FormalizedAt             string `json:"formalized_at"`
	WonAt                    string `json:"won_at"`
	LostAt                   string `json:"lost_at"`
}

type earlyFraudWarning struct {
	ID                 string `json:"id"`
	ProcessorID        string `json:"processor_id"`
	FraudType          string `json:"fraud_type"`
	ChargeRiskLevel    string `json:"charge_risk_level"`
	Actionable         bool   `json:"actionable"`
	Resolution         string `json:"resolution"`
	ResolutionMessage  string `json:"resolution_message"`
	ResolvedAt         string `json:"resolved_at"`
	ProcessorCreatedAt string `json:"processor_created_at"`
}

type purchaseAffiliateCredit struct {
	AmountCents     api.JSONInt `json:"amount_cents"`
	FeeCents        api.JSONInt `json:"fee_cents"`
	BasisPoints     api.JSONInt `json:"basis_points"`
	AffiliateUserID string      `json:"affiliate_user_id"`
}

type purchaseClusters struct {
	FingerprintCount *api.JSONInt `json:"fingerprint_count"`
	BrowserCount     *api.JSONInt `json:"browser_count"`
	IPCount          *api.JSONInt `json:"ip_count"`
}

type purchaseRenderOptions struct {
	ShowClusters bool
}

func newViewCmd() *cobra.Command {
	var withClusters bool

	cmd := &cobra.Command{
		Use:   "view <purchase-id>",
		Short: "View an admin purchase record",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			params := url.Values{}
			if withClusters {
				params.Set("with_clusters", "true")
			}
			path := cmdutil.JoinPath("purchases", args[0])
			return admincmd.RunGetDecoded[purchaseResponse](opts, "Fetching purchase...", path, params, func(resp purchaseResponse) error {
				return renderPurchaseWithOptions(opts, resp.Purchase, purchaseRenderOptions{ShowClusters: withClusters})
			})
		},
	}

	cmd.Flags().BoolVar(&withClusters, "with-clusters", false, "Include matching fingerprint, browser, and IP cluster counts")

	return cmd
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

func sellerEmail(p purchase) string {
	if p.Seller != nil && p.Seller.Email != "" {
		return p.Seller.Email
	}
	return p.SellerEmail
}

func SellerLabel(p Purchase) string {
	return sellerEmail(p)
}

func buyerLabel(p purchase) string {
	return p.Email
}

func sellerLabel(p purchase) string {
	if p.Seller != nil && (p.Seller.Email != "" || p.Seller.Name != "" || p.Seller.ID != "") {
		return userLabel(*p.Seller)
	}
	return p.SellerEmail
}

func userLabel(user purchaseSeller) string {
	parts := make([]string, 0, 3)
	if user.Email != "" {
		parts = append(parts, user.Email)
	}
	if user.Name != "" && user.Name != user.Email {
		parts = append(parts, user.Name)
	}
	if user.ID != "" {
		parts = append(parts, user.ID)
	}
	return strings.Join(parts, " / ")
}

func purchaseFlags(p purchase) string {
	flags := []string{}
	if p.ChargebackDate != "" {
		flags = append(flags, "CB")
	}
	if p.EarlyFraudWarning != nil {
		flags = append(flags, "EFW")
	}
	if p.CountryMismatches.hasAny() {
		flags = append(flags, "COUNTRY")
	}
	return strings.Join(flags, ",")
}

func RiskFlagsLabel(p Purchase) string {
	return purchaseFlags(p)
}

func (m countryMismatches) hasAny() bool {
	return m.BillingVsIP || m.BillingVsCard || m.IPVsCard
}

func renderPurchase(opts cmdutil.Options, p purchase) error {
	return renderPurchaseWithOptions(opts, p, purchaseRenderOptions{})
}

func renderPurchaseWithOptions(opts cmdutil.Options, p purchase, renderOpts purchaseRenderOptions) error {
	product := productLabel(p)
	amount := amountLabel(p)
	status := statusLabel(p)
	seller := sellerEmail(p)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{p.ID, p.Email, seller, product, amount, status, p.CreatedAt, p.ReceiptURL},
		})
	}
	return writePurchase(opts.Out(), opts.Style(), p, purchaseRenderOptions{
		ShowClusters: renderOpts.ShowClusters,
	})
}

func writePurchase(w io.Writer, style output.Styler, p purchase, renderOpts purchaseRenderOptions) error {
	product := productLabel(p)
	amount := amountLabel(p)
	status := statusLabel(p)
	seller := sellerEmail(p)

	headlineFromID := false
	headline := product
	if headline == "" {
		headline = p.ID
		headlineFromID = true
	}
	if amount != "" {
		headline += "  " + amount
	}
	if err := output.Writeln(w, style.Bold(headline)); err != nil {
		return err
	}
	if !headlineFromID {
		if err := output.Writef(w, "Purchase ID: %s\n", p.ID); err != nil {
			return err
		}
	}
	if p.Email != "" {
		if err := output.Writef(w, "Buyer: %s\n", p.Email); err != nil {
			return err
		}
	}
	if seller != "" {
		if err := output.Writef(w, "Seller: %s\n", seller); err != nil {
			return err
		}
	}
	if status != "" {
		if err := output.Writef(w, "Status: %s\n", status); err != nil {
			return err
		}
	}
	if p.CreatedAt != "" {
		if err := output.Writef(w, "Date: %s\n", p.CreatedAt); err != nil {
			return err
		}
	}
	if p.ReceiptURL != "" {
		if err := output.Writef(w, "Receipt: %s\n", p.ReceiptURL); err != nil {
			return err
		}
	}
	if err := writeRiskBlock(w, p); err != nil {
		return err
	}
	if credit := affiliateCreditSummary(p.AffiliateCredit); credit != "" {
		if err := output.Writef(w, "Affiliate credit: %s\n", credit); err != nil {
			return err
		}
	}
	if renderOpts.ShowClusters {
		return writeClusters(w, p.Clusters)
	}
	return nil
}

func writeRiskBlock(w io.Writer, p purchase) error {
	if !hasRiskDetails(p) {
		return nil
	}

	if err := output.Writeln(w, ""); err != nil {
		return err
	}
	if err := output.Writeln(w, "Risk:"); err != nil {
		return err
	}
	if p.CountryMismatches.hasAny() {
		if err := output.Writef(w, "  Country mismatches: %s", strings.Join(countryMismatchLabels(p.CountryMismatches), ", ")); err != nil {
			return err
		}
		countries := []string{}
		if p.BillingCountry != "" {
			countries = append(countries, "billing "+p.BillingCountry)
		}
		if p.IPCountry != "" {
			countries = append(countries, "IP "+p.IPCountry)
		}
		if p.CardCountry != "" {
			countries = append(countries, "card "+p.CardCountry)
		}
		if len(countries) > 0 {
			if err := output.Writef(w, " (%s)", strings.Join(countries, ", ")); err != nil {
				return err
			}
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
	}
	if card := cardSummary(p); card != "" {
		if err := output.Writef(w, "  Card: %s\n", card); err != nil {
			return err
		}
	}
	if p.ChargebackDate != "" {
		if err := output.Writef(w, "  Chargeback: %s\n", p.ChargebackDate); err != nil {
			return err
		}
	}
	if dispute := disputeSummary(p.Dispute); dispute != "" {
		if err := output.Writef(w, "  Dispute: %s\n", dispute); err != nil {
			return err
		}
	}
	if efw := earlyFraudWarningSummary(p.EarlyFraudWarning); efw != "" {
		if err := output.Writef(w, "  Early fraud warning: %s\n", efw); err != nil {
			return err
		}
	}
	if p.IPAddress != "" {
		ip := p.IPAddress
		if p.IPCountry != "" {
			ip += " (" + p.IPCountry + ")"
		}
		if err := output.Writef(w, "  IP: %s\n", ip); err != nil {
			return err
		}
	}
	if p.ChargeProcessor != "" {
		if err := output.Writef(w, "  Processor: %s\n", p.ChargeProcessor); err != nil {
			return err
		}
	}
	if p.PaypalOrderID != "" {
		if err := output.Writef(w, "  PayPal order: %s\n", p.PaypalOrderID); err != nil {
			return err
		}
	}
	return nil
}

func hasRiskDetails(p purchase) bool {
	return p.CountryMismatches.hasAny() ||
		cardSummary(p) != "" ||
		p.ChargebackDate != "" ||
		disputeSummary(p.Dispute) != "" ||
		earlyFraudWarningSummary(p.EarlyFraudWarning) != "" ||
		p.IPAddress != "" ||
		p.ChargeProcessor != "" ||
		p.PaypalOrderID != ""
}

func countryMismatchLabels(m countryMismatches) []string {
	labels := []string{}
	if m.BillingVsIP {
		labels = append(labels, "billing_vs_ip")
	}
	if m.BillingVsCard {
		labels = append(labels, "billing_vs_card")
	}
	if m.IPVsCard {
		labels = append(labels, "ip_vs_card")
	}
	return labels
}

func cardSummary(p purchase) string {
	parts := []string{}
	if p.Card.Visual != "" {
		parts = append(parts, p.Card.Visual)
	}
	if p.Card.Type != "" {
		parts = append(parts, p.Card.Type)
	}
	if p.Card.Bin != "" {
		parts = append(parts, "BIN "+p.Card.Bin)
	}
	if p.CardCountry != "" {
		parts = append(parts, "country "+p.CardCountry)
	}
	if p.Card.ExpiryMonth != 0 && p.Card.ExpiryYear != 0 {
		parts = append(parts, fmt.Sprintf("exp %02d/%d", p.Card.ExpiryMonth, p.Card.ExpiryYear))
	}
	return strings.Join(parts, ", ")
}

func disputeSummary(dispute *purchaseDispute) string {
	if dispute == nil {
		return ""
	}

	parts := []string{}
	if dispute.State != "" {
		parts = append(parts, dispute.State)
	}
	if dispute.Reason != "" {
		parts = append(parts, dispute.Reason)
	}
	if dispute.ChargeProcessorDisputeID != "" {
		parts = append(parts, dispute.ChargeProcessorDisputeID)
	} else if dispute.ID != "" {
		parts = append(parts, dispute.ID)
	}
	for _, timestamp := range []struct {
		label string
		value string
	}{
		{"formalized", dispute.FormalizedAt},
		{"initiated", dispute.InitiatedAt},
		{"won", dispute.WonAt},
		{"lost", dispute.LostAt},
		{"created", dispute.CreatedAt},
	} {
		if timestamp.value != "" {
			parts = append(parts, timestamp.label+" "+timestamp.value)
			break
		}
	}
	return strings.Join(parts, ", ")
}

func earlyFraudWarningSummary(efw *earlyFraudWarning) string {
	if efw == nil {
		return ""
	}

	parts := []string{}
	if efw.FraudType != "" {
		parts = append(parts, efw.FraudType)
	}
	if efw.ChargeRiskLevel != "" {
		parts = append(parts, "risk "+efw.ChargeRiskLevel)
	}
	if efw.Actionable {
		parts = append(parts, "actionable")
	}
	if efw.Resolution != "" {
		resolution := "resolution " + efw.Resolution
		if efw.ResolutionMessage != "" {
			resolution += ": " + efw.ResolutionMessage
		}
		parts = append(parts, resolution)
	} else if efw.ResolutionMessage != "" {
		parts = append(parts, "resolution message "+efw.ResolutionMessage)
	}
	if efw.ProcessorID != "" {
		parts = append(parts, efw.ProcessorID)
	}
	if efw.ResolvedAt != "" {
		parts = append(parts, "resolved "+efw.ResolvedAt)
	}
	if efw.ProcessorCreatedAt != "" {
		parts = append(parts, "processor created "+efw.ProcessorCreatedAt)
	}
	return strings.Join(parts, ", ")
}

func affiliateCreditSummary(credit *purchaseAffiliateCredit) string {
	if credit == nil {
		return ""
	}

	details := []string{}
	if credit.FeeCents != 0 {
		details = append(details, fmt.Sprintf("fee %d cents", credit.FeeCents))
	}
	if credit.BasisPoints != 0 {
		details = append(details, fmt.Sprintf("%d bps", credit.BasisPoints))
	}
	if credit.AffiliateUserID != "" {
		details = append(details, "affiliate "+credit.AffiliateUserID)
	}
	if credit.AmountCents == 0 && len(details) == 0 {
		return ""
	}

	summary := fmt.Sprintf("%d cents", credit.AmountCents)
	if len(details) > 0 {
		summary += " (" + strings.Join(details, ", ") + ")"
	}
	return summary
}

func writeClusters(w io.Writer, clusters *purchaseClusters) error {
	if clusters == nil {
		return nil
	}
	parts := []string{}
	if clusters.FingerprintCount != nil {
		parts = append(parts, fmt.Sprintf("fingerprint %d", *clusters.FingerprintCount))
	}
	if clusters.BrowserCount != nil {
		parts = append(parts, fmt.Sprintf("browser %d", *clusters.BrowserCount))
	}
	if clusters.IPCount != nil {
		parts = append(parts, fmt.Sprintf("IP %d", *clusters.IPCount))
	}
	if len(parts) == 0 {
		return nil
	}
	return output.Writef(w, "Clusters: %s\n", strings.Join(parts, ", "))
}
