package products

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	_ "unsafe"

	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

//go:linkname promptIsTerminal github.com/antiwork/gumroad-cli/internal/prompt.isTerminal
var promptIsTerminal func(int) bool

const (
	testSellerID    = "2245593582708"
	testSellerEmail = "seller@example.com"
	testProductID   = "abc123"
	testHoldID      = "hold_old"
	testNewerHoldID = "hold_newer"

	releaseLeavesListingUnchangedHint = "Releasing does not republish the listing. Bans, deletion and other admin restrictions are unchanged."
)

type capturedSalesHoldRequest struct {
	Method      string
	EscapedPath string
	RawQuery    string
	Auth        string
	ContentType string
	Body        map[string]any
}

func captureSalesHoldRequest(t *testing.T, r *http.Request) capturedSalesHoldRequest {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body %q: %v", raw, err)
	}
	return capturedSalesHoldRequest{
		Method:      r.Method,
		EscapedPath: r.URL.EscapedPath(),
		RawQuery:    r.URL.RawQuery,
		Auth:        r.Header.Get("Authorization"),
		ContentType: r.Header.Get("Content-Type"),
		Body:        body,
	}
}

func salesHoldPayload(status string, hold, product map[string]any) map[string]any {
	return map[string]any{
		"success":    true,
		"status":     status,
		"sales_hold": hold,
		"product":    product,
	}
}

func activeHold(id string) map[string]any {
	return map[string]any{
		"id":                  id,
		"active":              true,
		"unpublished_listing": true,
		"held_at":             "2026-10-26T12:00:00Z",
		"held_by_id":          "admin_1",
		"released_at":         nil,
		"released_by_id":      nil,
	}
}

func releasedHold(id string) map[string]any {
	hold := activeHold(id)
	hold["active"] = false
	hold["released_at"] = "2026-10-26T13:00:00Z"
	hold["released_by_id"] = "admin_2"
	return hold
}

func heldProduct(activeHoldID any) map[string]any {
	return map[string]any{
		"id":                   testProductID,
		"alive":                true,
		"published":            false,
		"draft":                false,
		"purchase_disabled_at": "2026-10-26T12:00:00Z",
		"deleted_at":           nil,
		"banned_at":            nil,
		"unpublished_by_admin": false,
		"active_sales_hold_id": activeHoldID,
	}
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": message})
}

func declinedConfirmationInput(t *testing.T) *os.File {
	t.Helper()

	oldIsTerminal := promptIsTerminal
	promptIsTerminal = func(int) bool { return true }
	t.Cleanup(func() { promptIsTerminal = oldIsTerminal })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe declined confirmation: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("write declined confirmation: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close declined confirmation writer: %v", err)
	}
	return r
}

func setupUnreachableSalesHoldAPI(t *testing.T) {
	t.Helper()
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	})
}

func assertUsageError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected usage error containing %q", want)
	}
	var usageErr *cmdutil.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

type salesHoldCommand struct {
	name    string
	newCmd  func() *cobra.Command
	args    []string
	path    string
	payload map[string]any
}

func salesHoldCommands() []salesHoldCommand {
	return []salesHoldCommand{
		{
			name:    "hold-sales",
			newCmd:  newHoldSalesCmd,
			args:    []string{testProductID, "--user-id", testSellerID},
			path:    "/internal/admin/products/abc123/hold_sales",
			payload: salesHoldPayload("held", activeHold(testHoldID), heldProduct(testHoldID)),
		},
		{
			name:    "release-sales-hold",
			newCmd:  newReleaseSalesHoldCmd,
			args:    []string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID},
			path:    "/internal/admin/products/abc123/release_sales_hold",
			payload: salesHoldPayload("released", releasedHold(testHoldID), heldProduct(nil)),
		},
	}
}

func TestSalesHoldCommandsRejectMissingOrBlankInputBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		want string
	}{
		{"hold missing product", newHoldSalesCmd, []string{"--user-id", testSellerID}, "missing required argument: <product-id>"},
		{"hold blank product", newHoldSalesCmd, []string{"  ", "--user-id", testSellerID}, "product ID must not be blank"},
		{"hold missing user", newHoldSalesCmd, []string{testProductID}, "missing required flag: --user-id"},
		{"hold blank user", newHoldSalesCmd, []string{testProductID, "--user-id", "  "}, "missing required flag: --user-id"},
		{"hold blank expected email", newHoldSalesCmd, []string{testProductID, "--user-id", testSellerID, "--expected-email", " "}, "--expected-email must not be blank"},
		{"hold blank email alias", newHoldSalesCmd, []string{testProductID, "--user-id", testSellerID, "--email", " "}, "--expected-email must not be blank"},
		{"hold mismatched user alias", newHoldSalesCmd, []string{testProductID, "--user-id", testSellerID, "--external-id", "other"}, "--user-id and --external-id must match"},
		{"hold extra argument", newHoldSalesCmd, []string{testProductID, "other", "--user-id", testSellerID}, "unexpected argument: other"},
		{"release missing product", newReleaseSalesHoldCmd, []string{"--user-id", testSellerID, "--hold-id", testHoldID}, "missing required argument: <product-id>"},
		{"release blank product", newReleaseSalesHoldCmd, []string{"", "--user-id", testSellerID, "--hold-id", testHoldID}, "product ID must not be blank"},
		{"release missing user", newReleaseSalesHoldCmd, []string{testProductID, "--hold-id", testHoldID}, "missing required flag: --user-id"},
		{"release missing hold", newReleaseSalesHoldCmd, []string{testProductID, "--user-id", testSellerID}, "missing required flag: --hold-id"},
		{"release blank hold", newReleaseSalesHoldCmd, []string{testProductID, "--user-id", testSellerID, "--hold-id", "  "}, "missing required flag: --hold-id"},
		{"release blank expected email", newReleaseSalesHoldCmd, []string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID, "--expected-email", ""}, "--expected-email must not be blank"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupUnreachableSalesHoldAPI(t)
			cmd := testutil.Command(tc.cmd(), testutil.Yes(true))
			cmd.SetArgs(tc.args)
			assertUsageError(t, cmd.Execute(), tc.want)
		})
	}
}

func TestSalesHoldCommandsRequireConfirmationWithNoInput(t *testing.T) {
	for _, tc := range salesHoldCommands() {
		t.Run(tc.name, func(t *testing.T) {
			setupUnreachableSalesHoldAPI(t)
			cmd := testutil.Command(tc.newCmd(), testutil.NoInput(true))
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--yes") {
				t.Fatalf("expected confirmation error mentioning --yes, got %v", err)
			}
		})
	}
}

func TestSalesHoldCommandsCancelWhenConfirmationIsDeclined(t *testing.T) {
	cases := []struct {
		cmd        salesHoldCommand
		prompt     string
		cancelText string
	}{
		{salesHoldCommands()[0], "Hold new sales of product abc123 on user_id 2245593582708?", "Cancelled: hold sales of product abc123 on user_id 2245593582708."},
		{salesHoldCommands()[1], "Release sales hold hold_old on product abc123 for user_id 2245593582708?", "Cancelled: release sales hold hold_old on product abc123."},
	}

	for _, tc := range cases {
		t.Run(tc.cmd.name, func(t *testing.T) {
			setupUnreachableSalesHoldAPI(t)
			var stderr bytes.Buffer
			cmd := testutil.Command(tc.cmd.newCmd(), testutil.Quiet(false), testutil.Stdin(declinedConfirmationInput(t)), testutil.Stderr(&stderr))
			cmd.SetArgs(tc.cmd.args)
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if !strings.Contains(stderr.String(), tc.prompt) {
				t.Fatalf("prompt missing %q: %q", tc.prompt, stderr.String())
			}
			if !strings.Contains(out, tc.cancelText) {
				t.Fatalf("cancel output missing %q: %q", tc.cancelText, out)
			}
		})
	}
}

func TestSalesHoldCommandsDeclinedConfirmationReportsCancelledJSON(t *testing.T) {
	setupUnreachableSalesHoldAPI(t)
	cmd := testutil.Command(newHoldSalesCmd(), testutil.JSONOutput(), testutil.Stdin(declinedConfirmationInput(t)), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if resp["success"] != false || resp["cancelled"] != true || resp["id"] != testProductID {
		t.Fatalf("unexpected cancelled payload: %s", out)
	}
}

func TestSalesHoldConfirmationMessagesDescribeScope(t *testing.T) {
	hold := fmt.Sprintf(holdSalesConfirmationMessage, testProductID, testSellerID)
	for _, want := range []string{"unpublishes the listing", "blocks new checkouts", "not emailed", "rest of the account is unchanged"} {
		if !strings.Contains(hold, want) {
			t.Errorf("hold confirmation missing %q: %q", want, hold)
		}
	}
	release := fmt.Sprintf(releaseSalesHoldConfirmationMessage, testHoldID, testProductID, testSellerID)
	if !strings.Contains(release, "does not republish the listing") {
		t.Errorf("release confirmation must say it does not republish: %q", release)
	}
}

func TestHoldSalesSendsSellerIdentityInJSONBody(t *testing.T) {
	var got capturedSalesHoldRequest
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureSalesHoldRequest(t, r)
		testutil.JSON(t, w, salesHoldPayload("held", activeHold(testHoldID), heldProduct(testHoldID)))
	})

	cmd := testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{" abc123 ", "--user-id", " 2245593582708 ", "--expected-email", " seller@example.com "})
	testutil.MustExecute(t, cmd)

	if got.Method != http.MethodPost || got.EscapedPath != "/internal/admin/products/abc123/hold_sales" {
		t.Fatalf("got %s %s, want POST /internal/admin/products/abc123/hold_sales", got.Method, got.EscapedPath)
	}
	if got.RawQuery != "" {
		t.Fatalf("identity must not be sent in the query string, got %q", got.RawQuery)
	}
	if got.Auth != "Bearer admin-token" {
		t.Fatalf("got Authorization %q, want stored admin token", got.Auth)
	}
	if !strings.HasPrefix(got.ContentType, "application/json") {
		t.Fatalf("got Content-Type %q, want application/json", got.ContentType)
	}
	want := map[string]any{"user_id": testSellerID, "expected_email": testSellerEmail}
	if fmt.Sprint(got.Body) != fmt.Sprint(want) {
		t.Fatalf("got body %v, want exactly %v", got.Body, want)
	}
}

func TestHoldSalesOmitsExpectedEmailWhenNotGiven(t *testing.T) {
	var got capturedSalesHoldRequest
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureSalesHoldRequest(t, r)
		testutil.JSON(t, w, salesHoldPayload("held", activeHold(testHoldID), heldProduct(testHoldID)))
	})

	cmd := testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--external-id", testSellerID, "--email", testSellerEmail})
	testutil.MustExecute(t, cmd)

	want := map[string]any{"user_id": testSellerID, "expected_email": testSellerEmail}
	if fmt.Sprint(got.Body) != fmt.Sprint(want) {
		t.Fatalf("aliases must map to the same body, got %v", got.Body)
	}

	cmd = testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID})
	testutil.MustExecute(t, cmd)

	if _, ok := got.Body["expected_email"]; ok || got.Body["user_id"] != testSellerID || len(got.Body) != 1 {
		t.Fatalf("got body %v, want only user_id", got.Body)
	}
}

func TestHoldSalesEscapesProductIDInPath(t *testing.T) {
	var got capturedSalesHoldRequest
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureSalesHoldRequest(t, r)
		testutil.JSON(t, w, salesHoldPayload("held", activeHold(testHoldID), heldProduct(testHoldID)))
	})

	cmd := testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{"a/b?c", "--user-id", testSellerID})
	testutil.MustExecute(t, cmd)

	if got.EscapedPath != "/internal/admin/products/a%2Fb%3Fc/hold_sales" || got.RawQuery != "" {
		t.Fatalf("product ID was not escaped as one path segment: path=%q query=%q", got.EscapedPath, got.RawQuery)
	}
}

func TestReleaseSalesHoldSendsExactHoldIDAndSellerIdentity(t *testing.T) {
	var got capturedSalesHoldRequest
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureSalesHoldRequest(t, r)
		testutil.JSON(t, w, salesHoldPayload("released", releasedHold(testHoldID), heldProduct(nil)))
	})

	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", " hold_old ", "--expected-email", testSellerEmail})
	testutil.MustExecute(t, cmd)

	if got.Method != http.MethodPost || got.EscapedPath != "/internal/admin/products/abc123/release_sales_hold" {
		t.Fatalf("got %s %s, want POST /internal/admin/products/abc123/release_sales_hold", got.Method, got.EscapedPath)
	}
	if got.RawQuery != "" || got.Auth != "Bearer admin-token" {
		t.Fatalf("unexpected query %q or auth %q", got.RawQuery, got.Auth)
	}
	want := map[string]any{"user_id": testSellerID, "hold_id": testHoldID, "expected_email": testSellerEmail}
	if fmt.Sprint(got.Body) != fmt.Sprint(want) {
		t.Fatalf("got body %v, want exactly %v", got.Body, want)
	}
}

func TestSalesHoldDryRunPrintsRequestWithoutContactingAPI(t *testing.T) {
	cases := []struct {
		cmd  salesHoldCommand
		want []string
	}{
		{salesHoldCommands()[0], []string{"Dry run: POST /internal/admin/products/abc123/hold_sales", "user_id: 2245593582708", "expected_email: seller@example.com"}},
		{salesHoldCommands()[1], []string{"Dry run: POST /internal/admin/products/abc123/release_sales_hold", "user_id: 2245593582708", "hold_id: hold_old", "expected_email: seller@example.com"}},
	}

	for _, tc := range cases {
		t.Run(tc.cmd.name, func(t *testing.T) {
			setupUnreachableSalesHoldAPI(t)
			var stderr bytes.Buffer
			cmd := testutil.Command(tc.cmd.newCmd(), testutil.DryRun(true), testutil.NoInput(true), testutil.Quiet(false), testutil.Stderr(&stderr))
			cmd.SetArgs(append(append([]string{}, tc.cmd.args...), "--expected-email", testSellerEmail))
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("dry-run output missing %q: %q", want, out)
				}
			}
			if strings.Contains(stderr.String(), "Admin actor:") {
				t.Errorf("dry-run must not resolve the mutation token, got %q", stderr.String())
			}
		})
	}
}

func TestSalesHoldDryRunJSONDescribesRequest(t *testing.T) {
	setupUnreachableSalesHoldAPI(t)
	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		DryRun bool                `json:"dry_run"`
		Method string              `json:"method"`
		Path   string              `json:"path"`
		Params map[string][]string `json:"params"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if !resp.DryRun || resp.Method != "POST" || resp.Path != "/internal/admin/products/abc123/release_sales_hold" {
		t.Fatalf("unexpected dry-run request: %s", out)
	}
	if fmt.Sprint(resp.Params) != fmt.Sprint(map[string][]string{"hold_id": {testHoldID}, "user_id": {testSellerID}}) {
		t.Fatalf("unexpected dry-run params: %v", resp.Params)
	}
}

func TestHoldSalesHumanOutputShowsHoldIDAndProductState(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("held", activeHold(testHoldID), heldProduct(testHoldID)))
	})

	var stderr bytes.Buffer
	cmd := testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(&stderr))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Sales held on product abc123",
		"Status: held",
		"Hold ID: hold_old",
		"Hold active: yes",
		"Held at: 2026-10-26T12:00:00Z",
		"Unpublished by this hold: yes",
		"Product active hold: hold_old",
		"Product published: no",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "Released at:") || strings.Contains(out, "still held") {
		t.Errorf("held output must not describe a release: %q", out)
	}
	if !strings.Contains(stderr.String(), "Admin actor: Test Admin (admin@example.com)") {
		t.Errorf("expected stored admin actor banner, got %q", stderr.String())
	}
}

func TestHoldSalesReportsExistingActiveHold(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("already_held", activeHold(testNewerHoldID), heldProduct(testNewerHoldID)))
	})

	cmd := testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Product abc123 already had an active sales hold", "Status: already_held", "Hold ID: hold_newer"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestReleaseSalesHoldOfOlderHoldShowsNewerActiveHold(t *testing.T) {
	payload := salesHoldPayload("released", releasedHold(testHoldID), heldProduct(testNewerHoldID))
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Released sales hold hold_old on product abc123",
		"Hold active: no",
		"Released at: 2026-10-26T13:00:00Z",
		"Product active hold: hold_newer",
		"Product abc123 is still held by a different active sales hold hold_newer; new sales stay blocked.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "Product active hold: none") {
		t.Errorf("output must not claim the product is unheld: %q", out)
	}
}

func TestReleaseSalesHoldQuietStillWarnsAboutNewerActiveHold(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("already_released", releasedHold(testHoldID), heldProduct(testNewerHoldID)))
	})

	var stderr bytes.Buffer
	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.NonInteractive(true), testutil.Stderr(&stderr))
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Fatalf("quiet mode must not write to stdout, got %q", out)
	}
	if !strings.Contains(stderr.String(), "still held by a different active sales hold hold_newer") {
		t.Fatalf("quiet mode must still warn about the remaining hold, got %q", stderr.String())
	}
}

func TestReleaseSalesHoldQuietIsSilentWhenNoOtherHoldRemains(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("released", releasedHold(testHoldID), heldProduct(nil)))
	})

	var stderr bytes.Buffer
	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.NonInteractive(true), testutil.Stderr(&stderr))
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" || stderr.String() != "" {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", out, stderr.String())
	}
}

func TestReleaseSalesHoldSaysListingStaysUnpublished(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("already_released", releasedHold(testHoldID), heldProduct(nil)))
	})

	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Sales hold hold_old on product abc123 was already released",
		"Status: already_released",
		"Product active hold: none",
		"Product published: no",
		releaseLeavesListingUnchangedHint,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestReleaseSalesHoldDoesNotClaimSellerCanRepublishRestrictedProduct(t *testing.T) {
	for _, tc := range []struct {
		name     string
		restrict func(map[string]any)
	}{
		{"banned", func(p map[string]any) { p["banned_at"] = "2026-10-26T11:00:00Z" }},
		{"unpublished by admin", func(p map[string]any) { p["unpublished_by_admin"] = true }},
		{"deleted", func(p map[string]any) {
			p["alive"] = false
			p["deleted_at"] = "2026-10-26T11:00:00Z"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			product := heldProduct(nil)
			tc.restrict(product)
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				testutil.JSON(t, w, salesHoldPayload("released", releasedHold(testHoldID), product))
			})

			cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(io.Discard))
			cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if strings.Contains(out, "seller publishes") {
				t.Errorf("output claims the seller can publish a restricted product: %q", out)
			}
			if !strings.Contains(out, releaseLeavesListingUnchangedHint) {
				t.Errorf("output missing %q: %q", releaseLeavesListingUnchangedHint, out)
			}
		})
	}
}

func TestSalesHoldUnknownStatusIsRenderedVerbatim(t *testing.T) {
	product := heldProduct(nil)
	product["published"] = true
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("queued_for_review", releasedHold(testHoldID), product))
	})

	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Sales hold hold_old on product abc123", "Status: queued_for_review", "Product published: yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "stays unpublished") {
		t.Errorf("published listing must not be described as unpublished: %q", out)
	}
}

func TestSalesHoldJSONPreservesServerResponse(t *testing.T) {
	payload := salesHoldPayload("released", releasedHold(testHoldID), heldProduct(testNewerHoldID))
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.JSONOutput(), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	wantJSON, _ := json.Marshal(payload)
	var want map[string]any
	_ = json.Unmarshal(wantJSON, &want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("JSON output changed the server response:\n got %v\nwant %v", got, want)
	}
}

func TestSalesHoldJQSelectsActiveHold(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, salesHoldPayload("released", releasedHold(testHoldID), heldProduct(testNewerHoldID)))
	})

	cmd := testutil.Command(newReleaseSalesHoldCmd(), testutil.Yes(true), testutil.JQ(".product.active_sales_hold_id"), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID, "--hold-id", testHoldID})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != `"hold_newer"` {
		t.Fatalf("got %q, want the newer active hold ID", out)
	}
}

func TestSalesHoldPlainOutput(t *testing.T) {
	cases := []struct {
		cmd     salesHoldCommand
		payload map[string]any
		want    string
	}{
		{salesHoldCommands()[0], salesHoldPayload("held", activeHold(testHoldID), heldProduct(testHoldID)), "held\tabc123\thold_old\ttrue\thold_old\tfalse"},
		{salesHoldCommands()[1], salesHoldPayload("released", releasedHold(testHoldID), heldProduct(testNewerHoldID)), "released\tabc123\thold_old\tfalse\thold_newer\tfalse"},
		{salesHoldCommands()[1], salesHoldPayload("released", releasedHold(testHoldID), heldProduct(nil)), "released\tabc123\thold_old\tfalse\t\tfalse"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd.name, func(t *testing.T) {
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				testutil.JSON(t, w, tc.payload)
			})

			cmd := testutil.Command(tc.cmd.newCmd(), testutil.Yes(true), testutil.PlainOutput(), testutil.Stderr(io.Discard))
			cmd.SetArgs(tc.cmd.args)
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if strings.TrimSuffix(out, "\n") != tc.want {
				t.Fatalf("got %q, want %q", out, tc.want)
			}
		})
	}
}

func TestSalesHoldResponseWithoutHoldIDIsAnError(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "status": "held", "product": heldProduct(nil)})
	})

	cmd := testutil.Command(newHoldSalesCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stderr(io.Discard))
	cmd.SetArgs([]string{testProductID, "--user-id", testSellerID})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "without a sales hold ID") || !strings.Contains(err.Error(), "gumroad admin products view abc123 --json") {
		t.Fatalf("expected missing hold ID error, got %v", err)
	}
}

func TestSalesHoldRefusalsSurfaceServerMessageWithoutUncertainty(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		message string
		want    string
	}{
		{"missing user", http.StatusBadRequest, "user_id is required for mutating admin actions.", "user_id is required for mutating admin actions."},
		{"legacy token", http.StatusUnauthorized, "per-actor admin token is required", "Not authenticated."},
		{"scope", http.StatusForbidden, "token scope does not allow this endpoint", "token scope does not allow this endpoint"},
		{"unknown hold", http.StatusNotFound, "Sales hold not found", "Sales hold not found"},
		{"email guard", http.StatusConflict, "expected_email does not match the user's current email", "expected_email does not match the user's current email"},
		{"seller changed", http.StatusConflict, "The listing's seller changed; nothing was held.", "The listing's seller changed; nothing was held."},
		{"wrong seller", http.StatusUnprocessableEntity, "Product does not belong to user", "Product does not belong to user"},
		{"success false", http.StatusOK, "Product not found", "Product not found"},
	}

	for _, command := range salesHoldCommands() {
		for _, tc := range cases {
			t.Run(command.name+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					writeAPIError(w, tc.status, tc.message)
				})

				cmd := testutil.Command(command.newCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
				cmd.SetArgs(command.args)
				err := cmd.Execute()

				var apiErr *api.APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected APIError, got %T: %v", err, err)
				}
				if apiErr.StatusCode != tc.status || !strings.Contains(apiErr.Message, tc.want) {
					t.Fatalf("got status %d message %q, want %d containing %q", apiErr.StatusCode, apiErr.Message, tc.status, tc.want)
				}
				if strings.Contains(err.Error(), "result is unknown") {
					t.Fatalf("a definite refusal must not be reported as uncertain: %v", err)
				}
				if calls.Load() != 1 {
					t.Fatalf("got %d requests, want 1", calls.Load())
				}
			})
		}
	}
}

func TestSalesHoldServerErrorsWarnThatResultIsUnknownWithoutResending(t *testing.T) {
	statuses := []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, command := range salesHoldCommands() {
		for _, status := range statuses {
			t.Run(fmt.Sprintf("%s/%d", command.name, status), func(t *testing.T) {
				var calls atomic.Int32
				testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					writeAPIError(w, status, "upstream trouble")
				})

				cmd := testutil.Command(command.newCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
				cmd.SetArgs(command.args)
				err := cmd.Execute()

				var apiErr *api.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
					t.Fatalf("expected APIError with status %d, got %T: %v", status, err, err)
				}
				if apiErr.Hint != "" {
					t.Errorf("an unknown result must not carry a retry hint, got %q", apiErr.Hint)
				}
				for _, want := range []string{
					command.name + " request failed: upstream trouble",
					"result is unknown and the request was not resent",
					"gumroad admin products view abc123 --json",
				} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error missing %q: %v", want, err)
					}
				}
				if calls.Load() != 1 {
					t.Fatalf("got %d requests, want exactly 1", calls.Load())
				}
			})
		}
	}
}

func TestSalesHoldTransportFailureWarnsThatResultIsUnknownWithoutResending(t *testing.T) {
	for _, command := range salesHoldCommands() {
		t.Run(command.name, func(t *testing.T) {
			var calls atomic.Int32
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.ReadAll(r.Body)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Errorf("hijack: %v", err)
					return
				}
				_ = conn.Close()
			})

			cmd := testutil.Command(command.newCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
			cmd.SetArgs(command.args)
			err := cmd.Execute()

			if err == nil {
				t.Fatal("expected transport failure")
			}
			var apiErr *api.APIError
			if errors.As(err, &apiErr) {
				t.Fatalf("transport failure must not look like a server answer: %v", err)
			}
			for _, want := range []string{
				command.name + " request failed:",
				"result is unknown and the request was not resent",
				"gumroad admin products view abc123 --json",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error missing %q: %v", want, err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("got %d requests, want exactly 1", calls.Load())
			}
		})
	}
}

func TestSalesHoldAuthFailureBeforeSendingIsNotReportedAsUncertain(t *testing.T) {
	for _, command := range salesHoldCommands() {
		t.Run(command.name, func(t *testing.T) {
			setupUnreachableSalesHoldAPI(t)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")

			cmd := testutil.Command(command.newCmd(), testutil.Yes(true), testutil.Stderr(io.Discard))
			cmd.SetArgs(command.args)
			err := cmd.Execute()

			if !errors.Is(err, adminconfig.ErrNotAuthenticated) {
				t.Fatalf("expected stored-token policy error, got %v", err)
			}
			if strings.Contains(err.Error(), "result is unknown") {
				t.Fatalf("nothing was sent, so the error must not claim an unknown result: %v", err)
			}
		})
	}
}

func TestSalesHoldNonInteractiveUsesEnvTokenWithoutActorBanner(t *testing.T) {
	for _, command := range salesHoldCommands() {
		t.Run(command.name, func(t *testing.T) {
			var gotAuth string
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				testutil.JSON(t, w, command.payload)
			})
			t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")

			var stderr bytes.Buffer
			cmd := testutil.Command(command.newCmd(), testutil.Yes(true), testutil.NonInteractive(true), testutil.JSONOutput(), testutil.Stderr(&stderr))
			cmd.SetArgs(command.args)
			testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if gotAuth != "Bearer env-admin-token" {
				t.Fatalf("got Authorization %q, want env admin token", gotAuth)
			}
			if strings.Contains(stderr.String(), "Admin actor:") {
				t.Fatalf("non-interactive mode must not print the actor banner, got %q", stderr.String())
			}
		})
	}
}
