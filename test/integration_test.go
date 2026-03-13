package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	buildBinaryOnce sync.Once
	buildBinaryPath string
	buildBinaryErr  error
)

func buildBinary(t *testing.T) string {
	t.Helper()
	buildBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gumroad-test-bin-*")
		if err != nil {
			buildBinaryErr = err
			return
		}
		buildBinaryPath = filepath.Join(dir, "gumroad")
		cmd := exec.Command("go", "build", "-o", buildBinaryPath, "./cmd/gumroad")
		cmd.Dir = getRootDir(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildBinaryErr = fmt.Errorf("%w\n%s", err, out)
		}
	})
	if buildBinaryErr != nil {
		t.Fatalf("build failed: %v", buildBinaryErr)
	}
	return buildBinaryPath
}

func getRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

func setupConfig(t *testing.T) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "gumroad", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"access_token":"test-token"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	return cfgDir
}

func runGR(t *testing.T, bin string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGRWithInput(t *testing.T, bin string, env []string, input string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func newFailingSalesPaginationServer(t *testing.T, gotQueries *[]string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotQueries != nil {
			*gotQueries = append(*gotQueries, r.URL.RawQuery)
		}

		switch r.URL.Query().Get("page_key") {
		case "":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success":       true,
				"next_page_key": "page-2",
				"sales": []map[string]any{
					{
						"id":                    "sale_1",
						"email":                 "first@example.com",
						"product_name":          "Art Pack",
						"formatted_total_price": "$10",
						"created_at":            "2024-01-15",
					},
				},
			}); err != nil {
				t.Fatalf("encode first sales page: %v", err)
			}
		case "page-2":
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": "page two failed",
			}); err != nil {
				t.Fatalf("encode failing sales page: %v", err)
			}
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	}))
}

func assertRetriedSecondSalesPage(t *testing.T, gotQueries []string) {
	t.Helper()

	if len(gotQueries) < 2 {
		t.Fatalf("expected paginated requests, got %v", gotQueries)
	}
	if gotQueries[0] != "" {
		t.Fatalf("expected first request without page key, got %v", gotQueries)
	}
	for _, query := range gotQueries[1:] {
		if query != "page_key=page-2" {
			t.Fatalf("expected subsequent requests for page-2, got %v", gotQueries)
		}
	}
}

func TestVersion(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "--version")
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "gumroad version") {
		t.Errorf("expected version string, got %q", out)
	}
}

func TestHelp(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "--help")
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, out)
	}
	for _, cmd := range []string{"auth", "user", "products", "sales", "payouts", "subscribers", "licenses", "offer-codes", "webhooks", "completion"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help missing command %q", cmd)
		}
	}
}

func TestProductsHelpMentionsCreateUpdateLimitation(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "products", "--help")
	if err != nil {
		t.Fatalf("products --help failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "does not support creating or updating products") {
		t.Fatalf("products help should mention API limitation, got %q", out)
	}
}

func TestTopLevelSKUsCommandIsUnavailable(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "skus", "--help")
	if err == nil {
		t.Fatal("expected top-level skus command to be unavailable")
	}
	if !strings.Contains(out, "unknown command \"skus\" for \"gumroad\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestProductsViewMissingIDShowsUsage(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "products", "view")
	if err == nil {
		t.Fatal("expected error without product ID")
	}
	for _, want := range []string{
		"missing required argument: <id>",
		"Usage:",
		"gumroad products view <id>",
		"Examples:",
		"Run \"gumroad products view --help\" for more information.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestWebhooksListMissingResourceShowsUsage(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "webhooks", "list")
	if err == nil {
		t.Fatal("expected error without --resource")
	}
	for _, want := range []string{
		"missing required flag: --resource",
		"Usage:",
		"gumroad webhooks list",
		"Examples:",
		"gumroad webhooks list --resource sale",
		"Run \"gumroad webhooks list --help\" for more information.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestLeafHelpIncludesExamples(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "products", "view", "--help")
	if err != nil {
		t.Fatalf("products view --help failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Examples:",
		"gumroad products view <id>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	for _, unwanted := range []string{
		"gumroad products list",
		"gumroad products delete <id>",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("unexpected sibling example %q in %q", unwanted, out)
		}
	}
}

func TestNoArgCommandRejectsUnexpectedArgument(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "auth", "status", "extra")
	if err == nil {
		t.Fatal("expected error for unexpected argument")
	}
	for _, want := range []string{
		"unexpected argument: extra",
		"Usage:",
		"gumroad auth status",
		"Examples:",
		"gumroad auth status",
		"Run \"gumroad auth status --help\" for more information.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestUserJSONErrorWhenNotAuthenticated(t *testing.T) {
	bin := buildBinary(t)
	cfgDir := t.TempDir()
	env := []string{"XDG_CONFIG_HOME=" + cfgDir}

	out, err := runGR(t, bin, env, "user", "--json")
	if err == nil {
		t.Fatal("expected user --json to fail without authentication")
	}

	errorPayload := assertJSONErrorEnvelope(t, out)
	if errorPayload["type"] != "auth_error" {
		t.Fatalf("expected auth_error, got %v in %v", errorPayload["type"], errorPayload)
	}
	if errorPayload["code"] != "not_authenticated" {
		t.Fatalf("expected not_authenticated, got %v in %v", errorPayload["code"], errorPayload)
	}
}

func TestProductsViewMissingIDJSONError(t *testing.T) {
	bin := buildBinary(t)

	out, err := runGR(t, bin, nil, "products", "view", "--json")
	if err == nil {
		t.Fatal("expected products view --json to fail without product ID")
	}

	errorPayload := assertJSONErrorEnvelope(t, out)
	if errorPayload["type"] != "usage_error" {
		t.Fatalf("expected usage_error, got %v in %v", errorPayload["type"], errorPayload)
	}
	message, _ := errorPayload["message"].(string)
	if !strings.Contains(message, "missing required argument: <id>") {
		t.Fatalf("expected missing argument message, got %q", message)
	}
	if !strings.Contains(message, "Usage:") {
		t.Fatalf("expected usage block in %q", message)
	}
}

func TestUnknownFlagJSONError(t *testing.T) {
	bin := buildBinary(t)

	out, err := runGR(t, bin, nil, "--json", "--bogus")
	if err == nil {
		t.Fatal("expected unknown flag to fail")
	}

	errorPayload := assertJSONErrorEnvelope(t, out)
	if errorPayload["type"] != "usage_error" {
		t.Fatalf("expected usage_error, got %v in %v", errorPayload["type"], errorPayload)
	}
	message, _ := errorPayload["message"].(string)
	if !strings.Contains(message, "unknown flag: --bogus") {
		t.Fatalf("expected unknown flag message, got %q", message)
	}
}

func TestUserJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(401)
			if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Unauthorized"}); err != nil {
				t.Fatalf("encode unauthorized response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user": map[string]any{
				"name":        "Test User",
				"email":       "test@example.com",
				"profile_url": "https://gumroad.com/testuser",
			},
		}); err != nil {
			t.Fatalf("encode user response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	out, err := runGR(t, bin, env, "user", "--json")
	if err != nil {
		t.Fatalf("user --json failed: %v\n%s", err, out)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	user := resp["user"].(map[string]any)
	if user["email"] != "test@example.com" {
		t.Errorf("got email=%v, want test@example.com", user["email"])
	}
}

func TestUserJQ(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user": map[string]any{
				"name":  "Test",
				"email": "jq@example.com",
			},
		}); err != nil {
			t.Fatalf("encode jq response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	out, err := runGR(t, bin, env, "user", "--jq", ".user.email")
	if err != nil {
		t.Fatalf("user --jq failed: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != `"jq@example.com"` {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), `"jq@example.com"`)
	}
}

func TestProductsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products" {
			w.WriteHeader(404)
			if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Not found"}); err != nil {
				t.Fatalf("encode products not-found response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"products": []map[string]any{
				{"id": "prod1", "name": "Digital Art", "published": true, "formatted_price": "$10", "sales_count": 5},
				{"id": "prod2", "name": "E-Book", "published": false, "formatted_price": "$25", "sales_count": 0},
			},
		}); err != nil {
			t.Fatalf("encode products response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	// JSON mode
	out, err := runGR(t, bin, env, "products", "list", "--json")
	if err != nil {
		t.Fatalf("products list --json failed: %v\n%s", err, out)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	products := resp["products"].([]any)
	if len(products) != 2 {
		t.Errorf("got %d products, want 2", len(products))
	}

	// Plain mode
	out, err = runGR(t, bin, env, "products", "list", "--plain")
	if err != nil {
		t.Fatalf("products list --plain failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "prod1") || !strings.Contains(out, "Digital Art") {
		t.Errorf("plain output missing product data: %q", out)
	}
}

func TestSalesListWithFilters(t *testing.T) {
	var gotParams string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParams = r.URL.RawQuery
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"sales": []map[string]any{
				{"id": "sale1", "email": "buyer@example.com", "product_name": "Art", "formatted_total_price": "$10", "created_at": "2024-01-15"},
			},
		}); err != nil {
			t.Fatalf("encode sales response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	out, err := runGR(t, bin, env, "sales", "list", "--product", "p1", "--after", "2024-01-01", "--json")
	if err != nil {
		t.Fatalf("sales list failed: %v\n%s", err, out)
	}
	if !strings.Contains(gotParams, "product_id=p1") {
		t.Errorf("expected product_id param, got query: %q", gotParams)
	}
	if !strings.Contains(gotParams, "after=2024-01-01") {
		t.Errorf("expected after param, got query: %q", gotParams)
	}
}

func TestSalesListAllJSONFailureDoesNotLeakPartialOutput(t *testing.T) {
	var gotQueries []string
	srv := newFailingSalesPaginationServer(t, &gotQueries)
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	out, err := runGR(t, bin, env, "sales", "list", "--all", "--json")
	if err == nil {
		t.Fatal("expected sales list --all --json to fail")
	}
	if !strings.Contains(out, "page two failed") {
		t.Fatalf("expected page-two failure in output, got %q", out)
	}
	if strings.Contains(out, "sale_1") || strings.Contains(out, "first@example.com") {
		t.Fatalf("should not leak partial JSON output, got %q", out)
	}
	errorPayload := assertJSONErrorEnvelope(t, out)
	if errorPayload["type"] != "api_error" {
		t.Fatalf("expected api_error, got %v in %v", errorPayload["type"], errorPayload)
	}
	assertRetriedSecondSalesPage(t, gotQueries)
}

func TestSalesListAllJQFailureDoesNotLeakPartialOutput(t *testing.T) {
	srv := newFailingSalesPaginationServer(t, nil)
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	out, err := runGR(t, bin, env, "sales", "list", "--all", "--jq", ".sales[] | .email")
	if err == nil {
		t.Fatal("expected sales list --all --jq to fail")
	}
	if !strings.Contains(out, "page two failed") {
		t.Fatalf("expected page-two failure in output, got %q", out)
	}
	if strings.Contains(out, `"first@example.com"`) {
		t.Fatalf("should not leak partial jq output, got %q", out)
	}
	errorPayload := assertJSONErrorEnvelope(t, out)
	if errorPayload["type"] != "api_error" {
		t.Fatalf("expected api_error, got %v in %v", errorPayload["type"], errorPayload)
	}
}

func TestInvalidJQReturnsStructuredJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user": map[string]any{
				"email": "jq@example.com",
			},
		}); err != nil {
			t.Fatalf("encode jq response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	out, err := runGR(t, bin, env, "user", "--jq", ".user[")
	if err == nil {
		t.Fatal("expected invalid jq to fail")
	}

	errorPayload := assertJSONErrorEnvelope(t, out)
	if errorPayload["type"] != "usage_error" {
		t.Fatalf("expected usage_error, got %v in %v", errorPayload["type"], errorPayload)
	}
	if errorPayload["code"] != "invalid_jq" {
		t.Fatalf("expected invalid_jq, got %v in %v", errorPayload["code"], errorPayload)
	}
	message, _ := errorPayload["message"].(string)
	if !strings.Contains(message, "invalid jq expression") {
		t.Fatalf("expected invalid jq message, got %q", message)
	}
}

func TestLicenseVerifyUsesTopLevel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if r.PostForm.Get("increment_uses_count") != "false" {
			t.Error("expected increment_uses_count=false for --no-increment")
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"uses":    42,
			"purchase": map[string]any{
				"email":      "license@example.com",
				"product_id": "prod1",
			},
		}); err != nil {
			t.Fatalf("encode license response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	// Test formatted output shows correct use count
	out, err := runGR(t, bin, env, "licenses", "verify", "--product", "prod1", "--key", "ABC-123", "--no-increment", "--no-color")
	if err != nil {
		t.Fatalf("licenses verify failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Uses: 42") {
		t.Errorf("expected 'Uses: 42' in output, got: %q", out)
	}

	// Test JSON output includes top-level uses
	out, err = runGR(t, bin, env, "licenses", "verify", "--product", "prod1", "--key", "ABC-123", "--no-increment", "--json")
	if err != nil {
		t.Fatalf("licenses verify --json failed: %v\n%s", err, out)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if resp["uses"] != float64(42) {
		t.Errorf("JSON uses=%v, want 42", resp["uses"])
	}
}

func TestLoginDistinguishesAuthVsTransportErrors(t *testing.T) {
	// Server that returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Internal error"}); err != nil {
			t.Fatalf("encode login 500 response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := t.TempDir()
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	// Pipe a token via stdin
	cmd := exec.Command(bin, "auth", "login", "--no-color")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader("some-token\n")
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err == nil {
		t.Fatal("expected login to fail on 500")
	}
	// Should say "could not verify" not "invalid token"
	if strings.Contains(outStr, "invalid token") {
		t.Errorf("500 should not be reported as 'invalid token': %q", outStr)
	}
	if !strings.Contains(outStr, "could not verify") {
		t.Errorf("expected 'could not verify' message, got: %q", outStr)
	}
}

func TestLoginReportsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Unauthorized"}); err != nil {
			t.Fatalf("encode login 401 response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := t.TempDir()
	env := []string{"XDG_CONFIG_HOME=" + cfgDir, "GUMROAD_API_BASE_URL=" + srv.URL}

	cmd := exec.Command(bin, "auth", "login", "--no-color")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader("bad-token\n")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected login to fail on 401")
	}
	if !strings.Contains(string(out), "invalid token") {
		t.Errorf("401 should be reported as 'invalid token': %q", string(out))
	}
}

func TestProductsHelp(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "products", "--help")
	if err != nil {
		t.Fatalf("products help failed: %v\n%s", err, out)
	}
	for _, sub := range []string{"list", "view", "delete", "enable", "disable"} {
		if !strings.Contains(out, sub) {
			t.Errorf("products help missing subcommand %q", sub)
		}
	}
}

func TestSalesHelp(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "sales", "--help")
	if err != nil {
		t.Fatalf("sales help failed: %v\n%s", err, out)
	}
	for _, sub := range []string{"list", "view", "refund", "ship", "resend-receipt"} {
		if !strings.Contains(out, sub) {
			t.Errorf("sales help missing subcommand %q", sub)
		}
	}
}

func TestLicensesHelp(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "licenses", "--help")
	if err != nil {
		t.Fatalf("licenses help failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no-increment") {
		t.Error("licenses help should mention --no-increment")
	}
}

func TestWebhooksHelp(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "webhooks", "--help")
	if err != nil {
		t.Fatalf("webhooks help failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OAuth app") {
		t.Error("webhooks help should warn about OAuth app scope")
	}
}

func TestCompletionNoAuth(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "completion", "bash")
	if err != nil {
		t.Fatalf("completion failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "bash") {
		t.Error("expected bash completion output")
	}
}

func TestNoInputBlocksDestructive(t *testing.T) {
	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{"XDG_CONFIG_HOME=" + cfgDir}

	out, err := runGR(t, bin, env, "products", "delete", "abc", "--no-input")
	if err == nil {
		t.Fatal("expected error for destructive op without --yes and --no-input")
	}
	if !strings.Contains(out, "--yes") {
		t.Errorf("error should suggest --yes, got %q", out)
	}
}

func TestNoImageFlag(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "products", "view", "--no-image", "--help")
	if err != nil {
		t.Fatalf("products view --no-image --help failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--no-image") {
		t.Errorf("help output should mention --no-image: %s", out)
	}
}

func TestGlobalFlags(t *testing.T) {
	bin := buildBinary(t)
	out, err := runGR(t, bin, nil, "--help")
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, out)
	}
	for _, flag := range []string{"--json", "--plain", "--jq", "--quiet", "--dry-run", "--no-color", "--no-input", "--yes", "--no-image", "--debug"} {
		if !strings.Contains(out, flag) {
			t.Errorf("help missing global flag %q", flag)
		}
	}
}

func TestDebugEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user": map[string]any{
				"name":  "Debug User",
				"email": "debug@example.com",
			},
		}); err != nil {
			t.Fatalf("encode debug response: %v", err)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cfgDir := setupConfig(t)
	env := []string{
		"XDG_CONFIG_HOME=" + cfgDir,
		"GUMROAD_API_BASE_URL=" + srv.URL,
		"GUMROAD_DEBUG=1",
	}

	out, err := runGR(t, bin, env, "user")
	if err != nil {
		t.Fatalf("user with GUMROAD_DEBUG failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "DEBUG request method=GET") || !strings.Contains(out, "status=200") {
		t.Fatalf("expected debug output in combined streams, got: %q", out)
	}
	if !strings.Contains(out, "Debug User") {
		t.Fatalf("expected command output alongside debug output, got: %q", out)
	}
}

func assertJSONErrorEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("error output is not valid JSON: %v\n%s", err, out)
	}

	success, ok := payload["success"].(bool)
	if !ok {
		t.Fatalf("missing boolean success field in %v", payload)
	}
	if success {
		t.Fatalf("expected success=false in %v", payload)
	}

	errorPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object in %v", payload)
	}
	return errorPayload
}
