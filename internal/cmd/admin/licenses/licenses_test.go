package licenses

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestLookupUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotKey, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		var payload lookupRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotKey = payload.LicenseKey
		testutil.JSON(t, w, map[string]any{
			"license": map[string]any{
				"email": "buyer@example.com", "product_name": "Course", "purchase_id": "123",
				"uses": 2, "enabled": true,
			},
		})
	})

	cmd := testutil.Command(newLookupCmd())
	cmd.SetArgs([]string{"--key", "ABC-123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/licenses/lookup" {
		t.Fatalf("got %s %s, want POST /internal/admin/licenses/lookup", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("license key should not be sent in query string, got %q", gotQuery)
	}
	if gotKey != "ABC-123" {
		t.Fatalf("got license_key %q, want ABC-123", gotKey)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{"Course", "Purchase ID: 123", "Buyer: buyer@example.com", "Uses: 2", "Status: enabled"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "ABC-123") {
		t.Fatalf("human output should not echo license key: %q", out)
	}
}

func TestLookupReadsLicenseKeyFromStdin(t *testing.T) {
	var gotKey string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Fatalf("license key should not be sent in query string, got %q", r.URL.RawQuery)
		}
		var payload lookupRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotKey = payload.LicenseKey
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"email": "buyer@example.com", "product_id": "prod_123"},
			"uses":     1,
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.Stdin(strings.NewReader("PIPE-KEY\n")))
	cmd.SetArgs([]string{})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotKey != "PIPE-KEY" {
		t.Fatalf("got license_key %q, want PIPE-KEY", gotKey)
	}
}

func TestLookupJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"license": map[string]any{"purchase_id": "123", "uses": 2},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--key", "ABC-123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		License map[string]any `json:"license"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.License["purchase_id"] != "123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestLookupPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"id": "123", "email": "buyer@example.com", "link_name": "Course", "uses": 3},
		})
	})

	cmd := testutil.Command(newLookupCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--key", "ABC-123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "123\tbuyer@example.com\tCourse\t3" {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestLookupHumanOutputShowsZeroUses(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"license": map[string]any{"purchase_id": "123", "uses": 0},
		})
	})

	cmd := testutil.Command(newLookupCmd())
	cmd.SetArgs([]string{"--key", "ABC-123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "123\nUses: 0" {
		t.Fatalf("unexpected human output: %q", out)
	}
}

func TestLookupRejectsEmptyKeyFlag(t *testing.T) {
	cmd := newLookupCmd()
	cmd.SetArgs([]string{"--key", ""})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected empty key error")
	}
	if !strings.Contains(err.Error(), "--key cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewLicensesCmdWiresLookup(t *testing.T) {
	cmd := NewLicensesCmd()
	if cmd.Use != "licenses" {
		t.Fatalf("Use = %q, want licenses", cmd.Use)
	}
	if got := cmd.Commands(); len(got) != 1 || got[0].Use != "lookup" {
		t.Fatalf("unexpected subcommands: %#v", got)
	}
}

func TestLicenseStatusVariants(t *testing.T) {
	enabled := true
	disabled := true
	falseValue := false

	for _, tc := range []struct {
		name string
		in   license
		want string
	}{
		{name: "enabled", in: license{Enabled: &enabled}, want: "enabled"},
		{name: "explicit disabled", in: license{Disabled: &disabled}, want: "disabled"},
		{name: "enabled false", in: license{Enabled: &falseValue}, want: "disabled"},
		{name: "disabled false", in: license{Disabled: &falseValue}, want: "enabled"},
		{name: "unknown", in: license{}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := licenseStatus(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
