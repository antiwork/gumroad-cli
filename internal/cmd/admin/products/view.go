package products

import (
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type product struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	Description          string                 `json:"description"`
	PriceCents           api.JSONInt            `json:"price_cents"`
	CurrencyCode         string                 `json:"currency_code"`
	Permalink            string                 `json:"permalink"`
	LongURL              string                 `json:"long_url"`
	PreviewURL           string                 `json:"preview_url"`
	CreatedAt            string                 `json:"created_at"`
	DeletedAt            string                 `json:"deleted_at"`
	BannedAt             string                 `json:"banned_at"`
	PurchaseDisabledAt   string                 `json:"purchase_disabled_at"`
	Alive                bool                   `json:"alive"`
	IsAdult              bool                   `json:"is_adult"`
	BadCardCounter       api.JSONInt            `json:"bad_card_counter"`
	Taxonomy             productTaxonomy        `json:"taxonomy"`
	Affiliates           []productAffiliate     `json:"affiliates"`
	RecentChargebackRate *productChargebackRate `json:"recent_chargeback_rate"`
	Seller               productSeller          `json:"seller"`
	Files                []productFile          `json:"files"`
}

type productSeller struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type productTaxonomy struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	AncestryPath []string `json:"ancestry_path"`
}

type productAffiliate struct {
	ID             string               `json:"id"`
	Type           string               `json:"type"`
	AffiliateUser  productAffiliateUser `json:"affiliate_user"`
	BasisPoints    api.JSONInt          `json:"basis_points"`
	DestinationURL string               `json:"destination_url"`
	Alive          bool                 `json:"alive"`
	DeletedAt      string               `json:"deleted_at"`
}

type productAffiliateUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type productChargebackRate struct {
	WindowDays       api.JSONInt `json:"window_days"`
	SuccessfulCount  api.JSONInt `json:"successful_count"`
	ChargedbackCount api.JSONInt `json:"chargedback_count"`
	Rate             *float64    `json:"rate"`
}

type productFile struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	FileName    string      `json:"file_name"`
	Extension   string      `json:"extension"`
	Filegroup   string      `json:"filegroup"`
	FileSize    api.JSONInt `json:"file_size"`
	CreatedAt   string      `json:"created_at"`
	DeletedAt   string      `json:"deleted_at"`
}

type viewResponse struct {
	Product product `json:"product"`
}

func newViewCmd() *cobra.Command {
	var withFraudContext bool

	cmd := &cobra.Command{
		Use:   "view <product-id>",
		Short: "View an admin product record",
		Example: `  gumroad admin products view abc123
  gumroad admin products view abc123 --with-fraud-context
  gumroad admin products view abc123 --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath("products", args[0])
			params := url.Values{}
			if withFraudContext {
				params.Set("with_fraud_context", "true")
			}
			return admincmd.RunGetDecoded[viewResponse](opts, "Fetching product...", path, params, func(resp viewResponse) error {
				return renderProduct(opts, resp.Product)
			})
		},
	}

	cmd.Flags().BoolVar(&withFraudContext, "with-fraud-context", false, "Request expensive fraud context such as recent chargeback rate")

	return cmd
}

func renderProduct(opts cmdutil.Options, p product) error {
	if opts.PlainOutput {
		return writeProductPlain(opts.Out(), p)
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writef(w, "%s", buildProductHeader(style, p)); err != nil {
			return err
		}
		if err := writeLifecycleSection(w, style, p); err != nil {
			return err
		}
		if err := writeRiskSection(w, style, p); err != nil {
			return err
		}
		if err := writeAffiliatesSection(w, style, p.Affiliates); err != nil {
			return err
		}
		return writeFilesSection(w, style, p.Files)
	})
}

func buildProductHeader(style output.Styler, p product) string {
	var b strings.Builder

	headline := productNameWithDeleted(p)
	if headline == "" {
		headline = p.ID
	}
	fmt.Fprintln(&b, style.Bold(headline))
	fmt.Fprintf(&b, "ID: %s\n", p.ID)
	if p.Permalink != "" && p.Permalink != p.ID {
		fmt.Fprintf(&b, "Permalink: %s\n", p.Permalink)
	}
	if p.LongURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", p.LongURL)
	}
	if label := sellerLabel(p.Seller); label != "" {
		fmt.Fprintf(&b, "Seller: %s\n", label)
	}
	fmt.Fprintf(&b, "Price: %s\n", formatPrice(p.PriceCents, p.CurrencyCode))
	fmt.Fprintf(&b, "Status: %s\n", productStatusLabel(p))
	if p.IsAdult {
		fmt.Fprintln(&b, "Adult: true")
	}
	if path := taxonomyPath(p.Taxonomy); path != "" {
		fmt.Fprintf(&b, "Taxonomy: %s\n", path)
	}
	if p.Description != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, style.Bold("Description:"))
		fmt.Fprintln(&b, p.Description)
	}
	fmt.Fprintln(&b)

	return b.String()
}

func writeProductPlain(w io.Writer, p product) error {
	return output.PrintPlain(w, [][]string{{
		p.ID,
		productNameWithDeleted(p),
		formatPrice(p.PriceCents, p.CurrencyCode),
		productStatusLabel(p),
		p.Seller.Email,
		strconv.Itoa(int(p.BadCardCounter)),
		taxonomyPath(p.Taxonomy),
		strconv.Itoa(len(p.Affiliates)),
		strconv.Itoa(len(p.Files)),
		p.CreatedAt,
		p.LongURL,
		formatChargebackRate(p.RecentChargebackRate),
	}})
}

func writeLifecycleSection(w io.Writer, style output.Styler, p product) error {
	if p.CreatedAt == "" && p.DeletedAt == "" && p.BannedAt == "" && p.PurchaseDisabledAt == "" {
		return nil
	}

	if err := output.Writef(w, "%s\n", style.Bold("Lifecycle:")); err != nil {
		return err
	}
	for _, row := range []struct {
		label string
		value string
	}{
		{"Created", p.CreatedAt},
		{"Deleted", p.DeletedAt},
		{"Banned", p.BannedAt},
		{"Purchase disabled", p.PurchaseDisabledAt},
	} {
		if row.value == "" {
			continue
		}
		if err := output.Writef(w, "  %s: %s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return output.Writeln(w, "")
}

func writeRiskSection(w io.Writer, style output.Styler, p product) error {
	if err := output.Writef(w, "%s\n", style.Bold("Risk:")); err != nil {
		return err
	}
	if err := output.Writef(w, "  Bad-card counter: %d\n", p.BadCardCounter); err != nil {
		return err
	}
	if rate := formatChargebackRate(p.RecentChargebackRate); rate != "" {
		if err := output.Writef(w, "  Recent chargeback rate: %s\n", rate); err != nil {
			return err
		}
	}
	return output.Writeln(w, "")
}

func writeAffiliatesSection(w io.Writer, style output.Styler, affiliates []productAffiliate) error {
	if len(affiliates) == 0 {
		return output.Writef(w, "%s none\n\n", style.Bold("Affiliates:"))
	}

	if err := output.Writef(w, "%s\n", style.Bold(fmt.Sprintf("Affiliates (%d):", len(affiliates)))); err != nil {
		return err
	}
	tbl := output.NewStyledTable(style, "ID", "TYPE", "USER", "BPS", "ALIVE", "DELETED", "URL")
	for _, a := range affiliates {
		tbl.AddRow(
			a.ID,
			a.Type,
			affiliateUserLabel(a.AffiliateUser),
			strconv.Itoa(int(a.BasisPoints)),
			strconv.FormatBool(a.Alive),
			a.DeletedAt,
			a.DestinationURL,
		)
	}
	if err := tbl.Render(w); err != nil {
		return err
	}
	return output.Writeln(w, "")
}

func writeFilesSection(w io.Writer, style output.Styler, files []productFile) error {
	if len(files) == 0 {
		return output.Writef(w, "%s none\n", style.Bold("Files:"))
	}
	header := style.Bold(fmt.Sprintf("Files (%d):", len(files))) + "\n"
	if err := output.Writef(w, "%s", header); err != nil {
		return err
	}
	tbl := output.NewStyledTable(style, "DISPLAY NAME", "FILE NAME", "EXT", "GROUP", "SIZE", "CREATED")
	for _, f := range files {
		tbl.AddRow(
			fileDisplayNameWithDeleted(f),
			f.FileName,
			f.Extension,
			f.Filegroup,
			formatFileSize(int(f.FileSize)),
			f.CreatedAt,
		)
	}
	return tbl.Render(w)
}

func fileDisplayNameWithDeleted(f productFile) string {
	name := f.DisplayName
	if name == "" {
		name = f.FileName
	}
	if f.DeletedAt != "" {
		if name == "" {
			return "(deleted)"
		}
		return name + " (deleted)"
	}
	return name
}

func sellerLabel(s productSeller) string {
	switch {
	case s.Email != "" && s.ID != "":
		return s.Email + " (" + s.ID + ")"
	case s.Email != "":
		return s.Email
	default:
		return s.ID
	}
}

func productStatusLabel(p product) string {
	if p.DeletedAt != "" {
		return "deleted"
	}
	if p.BannedAt != "" {
		return "banned"
	}
	if p.PurchaseDisabledAt != "" {
		return "purchase-disabled"
	}
	if p.Alive {
		return "alive"
	}
	return "unpublished"
}

func productNameWithDeleted(p product) string {
	if p.Name == "" {
		if p.DeletedAt != "" {
			return "(deleted)"
		}
		return ""
	}
	if p.DeletedAt != "" {
		return p.Name + " (deleted)"
	}
	return p.Name
}

func formatPrice(cents api.JSONInt, currency string) string {
	formatted := cmdutil.FormatMoney(int(cents), currency)
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "" {
		return formatted
	}
	return formatted + " " + code
}

func taxonomyPath(t productTaxonomy) string {
	if len(t.AncestryPath) > 0 {
		return strings.Join(t.AncestryPath, "/")
	}
	return t.Slug
}

func affiliateUserLabel(u productAffiliateUser) string {
	switch {
	case u.Email != "" && u.ID != "":
		return u.Email + " (" + u.ID + ")"
	case u.Email != "":
		return u.Email
	default:
		return u.ID
	}
}

func formatChargebackRate(rate *productChargebackRate) string {
	if rate == nil {
		return ""
	}

	percent := "n/a"
	if rate.Rate != nil {
		percent = fmt.Sprintf("%.2f%%", *rate.Rate*100)
	}
	return fmt.Sprintf(
		"%s (%d/%d over %dd)",
		percent,
		rate.ChargedbackCount,
		rate.SuccessfulCount,
		rate.WindowDays,
	)
}

const (
	bytesPerKiB    = 1024
	fileSizeDigits = 1
)

func formatFileSize(bytes int) string {
	if bytes < 0 {
		bytes = 0
	}
	if bytes < bytesPerKiB {
		return fmt.Sprintf("%d B", bytes)
	}
	size := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	var unit string
	for _, u := range units {
		size /= bytesPerKiB
		unit = u
		if displayedKiB(size) < float64(bytesPerKiB) {
			break
		}
	}
	return fmt.Sprintf("%.*f %s", fileSizeDigits, size, unit)
}

func displayedKiB(size float64) float64 {
	scale := math.Pow10(fileSizeDigits)
	return math.Round(size*scale) / scale
}
