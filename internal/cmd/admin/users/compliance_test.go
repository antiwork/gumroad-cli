package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestComplianceUsesInternalAdminEndpointAndRendersHumanOutput(t *testing.T) {
	var gotMethod, gotPath, gotEmail, gotAuth string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, individualCompliancePayload())
	})

	cmd := testutil.Command(newComplianceCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/compliance_info" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/compliance_info", gotMethod, gotPath)
	}
	if gotEmail != "seller@example.com" {
		t.Fatalf("got email %q, want seller@example.com", gotEmail)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}

	for _, want := range []string{
		"Compliance info for seller@example.com",
		"User ID: user_123",
		"Type: individual",
		"Legal name: Alice Investigator",
		"First name: Alice",
		"Last name: Investigator",
		"DBA: Alice & Co",
		"Birthday: 1985-06-07",
		"Nationality: US",
		"Phone: 5551234567",
		"Job title: Owner",
		"Individual address: address_full_match, San Francisco, California (CA), 94107, United States (US)",
		"Tax IDs:",
		"individual_last_four: ••••6789",
		"business_last_four: (not submitted)",
		"Identity docs:",
		"stripe_identity_document_id: idoc_individual",
		"stripe_company_document_id: (not submitted)",
		"Info requests:",
		"req_overdue",
		"individual_tax_id",
		"OVERDUE",
		"LAST EMAIL",
		"2026-05-02T00:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestComplianceRendersBusinessFields(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, businessCompliancePayload())
	})

	cmd := testutil.Command(newComplianceCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Compliance info for user_123",
		"Type: business",
		"Legal name: Acme LLC",
		"Business:",
		"Name: Acme LLC",
		"Type: llc",
		"Phone: 5550009999",
		"VAT ID: GB123456789",
		"Address: address_full_match, Burbank, California (CA), 91506, United States (US)",
		"individual_last_four: ••••0000",
		"business_last_four: ••••6789",
		"stripe_company_document_id: idoc_company",
		"stripe_additional_document_id: idoc_additional",
		"Info requests: none",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestCompliancePassesUserIDAndEmail(t *testing.T) {
	var gotEmail, gotUserID string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotEmail = r.URL.Query().Get("email")
		gotUserID = r.URL.Query().Get("user_id")
		testutil.JSON(t, w, map[string]any{
			"success":         true,
			"user_id":         "user_123",
			"compliance_info": nil,
			"info_requests":   []any{},
		})
	})

	cmd := testutil.Command(newComplianceCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--user-id", "user_123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotEmail != "seller@example.com" || gotUserID != "user_123" {
		t.Fatalf("got email=%q user_id=%q, want both forwarded", gotEmail, gotUserID)
	}
	if !strings.Contains(out, "No compliance info submitted for user_123") {
		t.Fatalf("unexpected no-info output: %q", out)
	}
}

func TestComplianceRequiresUserLookup(t *testing.T) {
	cmd := newComplianceCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email, --user-id, or --username") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestComplianceJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, individualCompliancePayload())
	})

	cmd := testutil.Command(newComplianceCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success        bool             `json:"success"`
		UserID         string           `json:"user_id"`
		ComplianceInfo map[string]any   `json:"compliance_info"`
		InfoRequests   []map[string]any `json:"info_requests"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "user_123" {
		t.Fatalf("unexpected JSON envelope: %s", out)
	}
	if resp.ComplianceInfo["legal_name"] != "Alice Investigator" {
		t.Fatalf("unexpected compliance_info JSON: %s", out)
	}
	if len(resp.InfoRequests) != 1 || resp.InfoRequests[0]["overdue"] != true {
		t.Fatalf("unexpected info_requests JSON: %s", out)
	}
}

func TestCompliancePlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, businessCompliancePayload())
	})

	cmd := testutil.Command(newComplianceCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "seller@example.com\tuser_123\tbusiness\tAcme LLC\tAcme LLC\t1985-06-07\tUS\t••••0000\t••••6789\t0"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestComplianceHighlightsOverdueRequestsWithColor(t *testing.T) {
	testutil.SetColorEnabled(t, true)
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, individualCompliancePayload())
	})

	cmd := testutil.Command(newComplianceCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "\x1b[31mOVERDUE\x1b[0m") {
		t.Fatalf("expected red overdue marker, got %q", out)
	}
}

func individualCompliancePayload() map[string]any {
	return map[string]any{
		"success": true,
		"user_id": "user_123",
		"compliance_info": map[string]any{
			"id":                     "uci_123",
			"is_business":            false,
			"legal_name":             "Alice Investigator",
			"first_name":             "Alice",
			"last_name":              "Investigator",
			"dba":                    "Alice & Co",
			"birthday":               "1985-06-07",
			"nationality":            "US",
			"phone":                  "5551234567",
			"job_title":              "Owner",
			"address":                complianceAddressFixture("San Francisco", "94107"),
			"business_name":          nil,
			"business_type":          nil,
			"business_phone":         nil,
			"business_vat_id_number": nil,
			"business_address":       nil,
			"tax_ids": map[string]any{
				"individual_last_four": "6789",
				"business_last_four":   nil,
			},
			"identity_documents": map[string]any{
				"stripe_identity_document_id":   "idoc_individual",
				"stripe_company_document_id":    nil,
				"stripe_additional_document_id": nil,
			},
			"created_at": "2026-05-01T12:00:00Z",
			"updated_at": "2026-05-02T12:00:00Z",
		},
		"info_requests": []map[string]any{
			{
				"id":                 "req_overdue",
				"field_needed":       "individual_tax_id",
				"state":              "requested",
				"due_at":             "2026-05-01T00:00:00Z",
				"overdue":            true,
				"created_at":         "2026-04-25T00:00:00Z",
				"last_email_sent_at": "2026-05-02T00:00:00Z",
			},
		},
	}
}

func businessCompliancePayload() map[string]any {
	return map[string]any{
		"success": true,
		"user_id": "user_123",
		"compliance_info": map[string]any{
			"id":                     "uci_456",
			"is_business":            true,
			"legal_name":             "Acme LLC",
			"first_name":             "Alice",
			"last_name":              "Investigator",
			"dba":                    "Acme",
			"birthday":               "1985-06-07",
			"nationality":            "US",
			"phone":                  "5551234567",
			"job_title":              "Owner",
			"address":                complianceAddressFixture("San Francisco", "94107"),
			"business_name":          "Acme LLC",
			"business_type":          "llc",
			"business_phone":         "5550009999",
			"business_vat_id_number": "GB123456789",
			"business_address":       complianceAddressFixture("Burbank", "91506"),
			"tax_ids": map[string]any{
				"individual_last_four": "0000",
				"business_last_four":   "6789",
			},
			"identity_documents": map[string]any{
				"stripe_identity_document_id":   nil,
				"stripe_company_document_id":    "idoc_company",
				"stripe_additional_document_id": "idoc_additional",
			},
			"created_at": "2026-05-01T12:00:00Z",
			"updated_at": "2026-05-02T12:00:00Z",
		},
		"info_requests": []any{},
	}
}

func complianceAddressFixture(city, zipCode string) map[string]any {
	return map[string]any{
		"street_address": "address_full_match",
		"city":           city,
		"state":          "California",
		"state_code":     "CA",
		"zip_code":       zipCode,
		"country":        "United States",
		"country_code":   "US",
	}
}
