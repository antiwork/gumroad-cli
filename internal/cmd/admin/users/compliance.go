package users

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type complianceResponse struct {
	UserID         string                  `json:"user_id"`
	ComplianceInfo *complianceInfo         `json:"compliance_info"`
	InfoRequests   []complianceInfoRequest `json:"info_requests"`
}

type complianceInfo struct {
	ID                  string                      `json:"id"`
	IsBusiness          bool                        `json:"is_business"`
	LegalName           string                      `json:"legal_name"`
	FirstName           string                      `json:"first_name"`
	LastName            string                      `json:"last_name"`
	DBA                 string                      `json:"dba"`
	Birthday            string                      `json:"birthday"`
	Nationality         string                      `json:"nationality"`
	Phone               string                      `json:"phone"`
	JobTitle            string                      `json:"job_title"`
	Address             complianceAddress           `json:"address"`
	BusinessName        string                      `json:"business_name"`
	BusinessType        string                      `json:"business_type"`
	BusinessPhone       string                      `json:"business_phone"`
	BusinessVATIDNumber string                      `json:"business_vat_id_number"`
	BusinessAddress     *complianceAddress          `json:"business_address"`
	TaxIDs              complianceTaxIDs            `json:"tax_ids"`
	IdentityDocuments   complianceIdentityDocuments `json:"identity_documents"`
	CreatedAt           string                      `json:"created_at"`
	UpdatedAt           string                      `json:"updated_at"`
}

type complianceAddress struct {
	StreetAddress string `json:"street_address"`
	City          string `json:"city"`
	State         string `json:"state"`
	StateCode     string `json:"state_code"`
	ZipCode       string `json:"zip_code"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code"`
}

type complianceTaxIDs struct {
	IndividualLastFour string `json:"individual_last_four"`
	BusinessLastFour   string `json:"business_last_four"`
}

type complianceIdentityDocuments struct {
	StripeIdentityDocumentID   string `json:"stripe_identity_document_id"`
	StripeCompanyDocumentID    string `json:"stripe_company_document_id"`
	StripeAdditionalDocumentID string `json:"stripe_additional_document_id"`
}

type complianceInfoRequest struct {
	ID              string `json:"id"`
	FieldNeeded     string `json:"field_needed"`
	State           string `json:"state"`
	DueAt           string `json:"due_at"`
	Overdue         bool   `json:"overdue"`
	CreatedAt       string `json:"created_at"`
	LastEmailSentAt string `json:"last_email_sent_at"`
}

func newComplianceCmd() *cobra.Command {
	var lookup userLookupFlags

	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "View a user's submitted compliance info",
		Long: `View a user's submitted KYC data and open compliance info requests.

Identify the user with --email, --user-id, or --username. When more than one
is supplied, the server resolves by --user-id first, then --email, then
--username.`,
		Example: `  gumroad admin users compliance --user-id 2245593582708
  gumroad admin users compliance --username sellerone
  gumroad admin users compliance --email user@example.com --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}

			return admincmd.RunGetDecoded[complianceResponse](opts, "Fetching compliance info...", "/users/compliance_info", target.Values(), func(resp complianceResponse) error {
				return renderCompliance(opts, target.Identifier(), resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)

	return cmd
}

func renderCompliance(opts cmdutil.Options, identifier string, resp complianceResponse) error {
	if opts.PlainOutput {
		return writeCompliancePlain(opts.Out(), identifier, resp)
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if resp.ComplianceInfo == nil {
			if err := output.Writef(w, "No compliance info submitted for %s\n", identifier); err != nil {
				return err
			}
			if resp.UserID != "" && resp.UserID != identifier {
				if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
					return err
				}
			}
			if len(resp.InfoRequests) > 0 {
				if err := output.Writeln(w, ""); err != nil {
					return err
				}
				return writeInfoRequestsSection(w, style, resp.InfoRequests)
			}
			return nil
		}

		if err := writeComplianceSummary(w, style, identifier, resp.UserID, *resp.ComplianceInfo); err != nil {
			return err
		}
		if err := writeBusinessSection(w, style, *resp.ComplianceInfo); err != nil {
			return err
		}
		if err := writeTaxIDsSection(w, style, resp.ComplianceInfo.TaxIDs); err != nil {
			return err
		}
		if err := writeIdentityDocumentsSection(w, style, resp.ComplianceInfo.IdentityDocuments); err != nil {
			return err
		}
		return writeInfoRequestsSection(w, style, resp.InfoRequests)
	})
}

func writeCompliancePlain(w io.Writer, identifier string, resp complianceResponse) error {
	if resp.ComplianceInfo == nil {
		return output.PrintPlain(w, [][]string{{
			identifier,
			resp.UserID,
			"none",
			"",
			"",
			"",
			"",
			"",
			"",
			strconv.Itoa(len(resp.InfoRequests)),
		}})
	}

	info := resp.ComplianceInfo
	return output.PrintPlain(w, [][]string{{
		identifier,
		resp.UserID,
		complianceEntityType(*info),
		info.LegalName,
		info.BusinessName,
		info.Birthday,
		info.Nationality,
		maskTaxID(info.TaxIDs.IndividualLastFour),
		maskTaxID(info.TaxIDs.BusinessLastFour),
		strconv.Itoa(len(resp.InfoRequests)),
	}})
}

func writeComplianceSummary(w io.Writer, style output.Styler, identifier, userID string, info complianceInfo) error {
	if err := output.Writef(w, "%s\n", style.Bold("Compliance info for "+identifier)); err != nil {
		return err
	}
	if userID != "" && userID != identifier {
		if err := output.Writef(w, "User ID: %s\n", userID); err != nil {
			return err
		}
	}

	for _, row := range []struct {
		label string
		value string
	}{
		{"Type", complianceEntityType(info)},
		{"Legal name", info.LegalName},
		{"First name", info.FirstName},
		{"Last name", info.LastName},
		{"DBA", info.DBA},
		{"Birthday", info.Birthday},
		{"Nationality", info.Nationality},
		{"Phone", info.Phone},
		{"Job title", info.JobTitle},
		{"Individual address", formatComplianceAddress(info.Address)},
		{"Created", info.CreatedAt},
		{"Updated", info.UpdatedAt},
	} {
		if row.value == "" {
			continue
		}
		if err := output.Writef(w, "%s: %s\n", row.label, row.value); err != nil {
			return err
		}
	}

	return output.Writeln(w, "")
}

func writeBusinessSection(w io.Writer, style output.Styler, info complianceInfo) error {
	if !info.IsBusiness {
		return nil
	}

	if err := output.Writef(w, "%s\n", style.Bold("Business:")); err != nil {
		return err
	}
	for _, row := range []struct {
		label string
		value string
	}{
		{"Name", info.BusinessName},
		{"Type", info.BusinessType},
		{"Phone", info.BusinessPhone},
		{"VAT ID", info.BusinessVATIDNumber},
		{"Address", formatOptionalComplianceAddress(info.BusinessAddress)},
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

func writeTaxIDsSection(w io.Writer, style output.Styler, taxIDs complianceTaxIDs) error {
	if err := output.Writef(w, "%s\n", style.Bold("Tax IDs:")); err != nil {
		return err
	}
	if err := output.Writef(w, "  individual_last_four: %s\n", submittedOrNone(maskTaxID(taxIDs.IndividualLastFour))); err != nil {
		return err
	}
	if err := output.Writef(w, "  business_last_four: %s\n", submittedOrNone(maskTaxID(taxIDs.BusinessLastFour))); err != nil {
		return err
	}
	return output.Writeln(w, "")
}

func writeIdentityDocumentsSection(w io.Writer, style output.Styler, docs complianceIdentityDocuments) error {
	if err := output.Writef(w, "%s\n", style.Bold("Identity docs:")); err != nil {
		return err
	}
	for _, row := range []struct {
		label string
		value string
	}{
		{"stripe_identity_document_id", docs.StripeIdentityDocumentID},
		{"stripe_company_document_id", docs.StripeCompanyDocumentID},
		{"stripe_additional_document_id", docs.StripeAdditionalDocumentID},
	} {
		if err := output.Writef(w, "  %s: %s\n", row.label, submittedOrNone(row.value)); err != nil {
			return err
		}
	}
	return output.Writeln(w, "")
}

func writeInfoRequestsSection(w io.Writer, style output.Styler, requests []complianceInfoRequest) error {
	if len(requests) == 0 {
		return output.Writef(w, "%s none\n", style.Bold("Info requests:"))
	}

	if err := output.Writef(w, "%s\n", style.Bold("Info requests:")); err != nil {
		return err
	}
	tbl := output.NewStyledTable(style, "ID", "FIELD", "STATE", "DUE", "STATUS", "CREATED", "LAST EMAIL")
	for _, request := range requests {
		tbl.AddRow(
			request.ID,
			request.FieldNeeded,
			request.State,
			request.DueAt,
			infoRequestStatus(style, request),
			request.CreatedAt,
			request.LastEmailSentAt,
		)
	}
	return tbl.Render(w)
}

func complianceEntityType(info complianceInfo) string {
	if info.IsBusiness {
		return "business"
	}
	return "individual"
}

func formatOptionalComplianceAddress(address *complianceAddress) string {
	if address == nil {
		return ""
	}
	return formatComplianceAddress(*address)
}

func formatComplianceAddress(address complianceAddress) string {
	parts := []string{
		address.StreetAddress,
		address.City,
		formatState(address.State, address.StateCode),
		address.ZipCode,
		formatCountry(address.Country, address.CountryCode),
	}

	return joinNonEmpty(parts, ", ")
}

func formatState(state, code string) string {
	return formatNameWithCode(state, code)
}

func formatCountry(country, code string) string {
	return formatNameWithCode(country, code)
}

func formatNameWithCode(name, code string) string {
	switch {
	case name != "" && code != "" && name != code:
		return fmt.Sprintf("%s (%s)", name, code)
	case name != "":
		return name
	default:
		return code
	}
}

func joinNonEmpty(values []string, sep string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, sep)
}

func maskTaxID(lastFour string) string {
	if lastFour == "" {
		return ""
	}
	return "••••" + lastFour
}

func submittedOrNone(value string) string {
	if value == "" {
		return "(not submitted)"
	}
	return value
}

func infoRequestStatus(style output.Styler, request complianceInfoRequest) string {
	if request.Overdue {
		return style.Red("OVERDUE")
	}
	return "open"
}
