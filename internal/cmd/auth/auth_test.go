package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/oauth"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

// syncBuffer is a thread-safe bytes.Buffer for concurrent test read/write.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func setupAuth(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	t.Cleanup(srv.Close)
}

func withConfig(t *testing.T, token string) {
	t.Helper()
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"`+token+`"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
}

func withEnvAccessToken(t *testing.T, token string) {
	t.Helper()
	t.Setenv(config.EnvAccessToken, token)
}

func runStatus(t *testing.T, mutators ...testutil.OptionsMutator) string {
	t.Helper()

	var out bytes.Buffer
	mutators = append(mutators, testutil.Stdout(&out))
	cmd := testutil.Command(newStatusCmd(), mutators...)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return out.String()
}

// --- Login ---

func TestLogin_401_ReportsInvalidToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Unauthorized"}); err != nil {
			t.Fatalf("encode unauthorized response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("bad-token\n")))

	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("401 should say 'invalid token', got: %v", err)
	}
}

func TestLogin_500_ReportsConnectionError(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Internal error"}); err != nil {
			t.Fatalf("encode internal error response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("some-token\n")))

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if strings.Contains(err.Error(), "invalid token") {
		t.Errorf("500 should NOT say 'invalid token': %v", err)
	}
	if !strings.Contains(err.Error(), "could not verify") {
		t.Errorf("500 should say 'could not verify': %v", err)
	}
}

func TestLogin_SavesToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(401)
			if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
				t.Fatalf("encode unauthorized response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Test", "email": "t@t.com"},
		}); err != nil {
			t.Fatalf("encode login success response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("good-token\n")))

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(cfgDir, "gumroad", "config.json"))
	if readErr != nil {
		t.Fatalf("config not saved: %v", readErr)
	}
	if !strings.Contains(string(data), "good-token") {
		t.Errorf("config should contain token, got: %s", data)
	}
}

func TestLogin_EmptyToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with empty token")
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("  \n")))

	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "token cannot be empty") {
		t.Fatalf("expected empty token error, got: %v", err)
	}
	for _, want := range []string{"Usage:", "gumroad auth login", "Examples:"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func TestLogin_403_ReportsInvalidToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Forbidden"}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("bad-token\n")))

	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("403 should say 'invalid token', got: %v", err)
	}
}

func TestLogin_ShowsUserInfo(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/login_success.json"))
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.Quiet(false), testutil.Stdout(&out), testutil.Stdin(strings.NewReader("good-token\n")))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if !strings.Contains(out.String(), "Jane") || !strings.Contains(out.String(), "jane@example.com") {
		t.Errorf("login should show user info: %q", out.String())
	}
}

func TestLogin_JSONOutput(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/login_success.json"))
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.JSONOutput(), testutil.Stdout(&out), testutil.Stdin(strings.NewReader("good-token\n")))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("login JSON output is invalid: %v\n%s", err, out.String())
	}
	if resp["authenticated"] != true {
		t.Fatalf("got authenticated=%v, want true", resp["authenticated"])
	}
	user := resp["user"].(map[string]any)
	if user["email"] != "jane@example.com" {
		t.Fatalf("got email=%v, want jane@example.com", user["email"])
	}
}

func TestLogin_PlainOutput(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/login_success.json"))
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.PlainOutput(), testutil.Stdout(&out), testutil.Stdin(strings.NewReader("good-token\n")))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "true\tJane\tjane@example.com" {
		t.Fatalf("unexpected plain output: %q", out.String())
	}
}

func TestLogin_DoesNotSaveTokenWhenResponseIsInvalid(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"user":`)
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("good-token\n")))

	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(cfgDir, "gumroad", "config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config should not be written on parse failure, got err=%v", statErr)
	}
}

func TestLogin_DryRunSkipsVerificationAndSave(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("login dry-run should not reach API")
	})
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "gumroad", "config.json")
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.DryRun(true), testutil.NoInput(true), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("config should not be written during dry-run, got err=%v", err)
	}
	if !strings.Contains(out.String(), "Dry run") || !strings.Contains(out.String(), "store API token") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}
}

// --- Status ---

func TestStatus_NotLoggedIn(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API when not logged in")
	})

	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	out := runStatus(t)
	if !strings.Contains(out, "Not logged in") {
		t.Errorf("should say 'Not logged in': %q", out)
	}
	if !strings.Contains(out, config.EnvAccessToken) {
		t.Errorf("status should mention %s: %q", config.EnvAccessToken, out)
	}
}

func TestStatus_InvalidToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "expired")

	out := runStatus(t)
	if !strings.Contains(out, "invalid or expired") {
		t.Errorf("should say 'invalid or expired': %q", out)
	}
}

func TestStatus_AccessDenied(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "restricted")

	out := runStatus(t)
	if !strings.Contains(out, "access is denied") {
		t.Errorf("should say 'access is denied': %q", out)
	}
}

func TestStatus_ServerError(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Server error"}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "tok")

	cmd := newStatusCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "could not verify") {
		t.Fatalf("expected 'could not verify', got: %v", err)
	}
}

func TestStatus_ValidToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")

	out := runStatus(t)
	if !strings.Contains(out, "Jane") || !strings.Contains(out, "jane@example.com") {
		t.Errorf("should show user info: %q", out)
	}
	if !strings.Contains(out, "Source: stored config") {
		t.Errorf("status should show config source: %q", out)
	}
}

func TestStatus_ShowsStoredAdminWhoami(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")
	if err := adminconfig.Save(&adminconfig.Config{
		Token:           "admin-token",
		TokenExternalID: "adm_local",
		Actor:           adminconfig.Actor{Name: "Cached Admin", Email: "cached@example.com"},
		ExpiresAt:       "cached",
	}); err != nil {
		t.Fatalf("Save admin config failed: %v", err)
	}
	adminSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/admin/whoami" {
			t.Fatalf("got path %q, want /internal/admin/whoami", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
			t.Fatalf("got Authorization=%q, want Bearer admin-token", got)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"actor":  map[string]any{"name": "Live Admin", "email": "admin@example.com"},
			"token":  map[string]any{"external_id": "adm_123", "expires_at": "2026-06-01T00:00:00Z"},
			"scopes": []string{"admin"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	}))
	t.Cleanup(adminSrv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, adminSrv.URL)

	out := runStatus(t)
	if !strings.Contains(out, "Live Admin") || !strings.Contains(out, "2026-06-01T00:00:00Z") {
		t.Fatalf("expected admin whoami output, got %q", out)
	}
}

func TestStatus_ValidTokenWithNameOnly(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")

	out := runStatus(t)
	if !strings.Contains(out, "Logged in as Jane") {
		t.Fatalf("expected name-only authenticated output, got %q", out)
	}
	if strings.Contains(out, "()") {
		t.Fatalf("should not show empty email placeholder, got %q", out)
	}
}

func TestStatus_ValidTokenWithEmailOnly(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")

	out := runStatus(t)
	if !strings.Contains(out, "Logged in as jane@example.com") {
		t.Fatalf("expected email-only authenticated output, got %q", out)
	}
	if strings.Contains(out, "()") {
		t.Fatalf("should not show empty email placeholder, got %q", out)
	}
}

func TestStatus_JSONOutput_Authenticated(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")
	out := runStatus(t, testutil.JSONOutput())

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("status JSON output is invalid: %v\n%s", err, out)
	}
	if resp["authenticated"] != true {
		t.Errorf("got authenticated=%v, want true", resp["authenticated"])
	}
	user := resp["user"].(map[string]any)
	if user["email"] != "jane@example.com" {
		t.Errorf("got email=%v, want jane@example.com", user["email"])
	}
	if resp["source"] != string(config.TokenSourceConfig) {
		t.Errorf("got source=%v, want %s", resp["source"], config.TokenSourceConfig)
	}
}

func TestStatus_JQOutput_Authenticated(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")
	out := runStatus(t, testutil.JQ(".authenticated"))

	if strings.TrimSpace(out) != "true" {
		t.Fatalf("got %q, want true", strings.TrimSpace(out))
	}
}

func TestStatus_ValidTokenWithoutUserFields(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")

	out := runStatus(t)
	if !strings.Contains(out, "Authenticated.") {
		t.Fatalf("expected fallback authenticated output, got %q", out)
	}
}

func TestStatus_JSONOutput_NotLoggedIn(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API when not logged in")
	})

	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	out := runStatus(t, testutil.JSONOutput())

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("status JSON output is invalid: %v\n%s", err, out)
	}
	if resp["authenticated"] != false {
		t.Errorf("got authenticated=%v, want false", resp["authenticated"])
	}
	if resp["reason"] != statusReasonNotLoggedIn {
		t.Errorf("got reason=%v, want %s", resp["reason"], statusReasonNotLoggedIn)
	}
}

func TestStatus_JSONOutput_InvalidToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "expired")
	out := runStatus(t, testutil.JSONOutput())

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("status JSON output is invalid: %v\n%s", err, out)
	}
	if resp["authenticated"] != false {
		t.Errorf("got authenticated=%v, want false", resp["authenticated"])
	}
	if resp["reason"] != statusReasonInvalidOrExpired {
		t.Errorf("got reason=%v, want %s", resp["reason"], statusReasonInvalidOrExpired)
	}
}

func TestStatus_JSONOutput_AccessDenied(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "restricted")
	out := runStatus(t, testutil.JSONOutput())

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("status JSON output is invalid: %v\n%s", err, out)
	}
	if resp["authenticated"] != false {
		t.Errorf("got authenticated=%v, want false", resp["authenticated"])
	}
	if resp["reason"] != statusReasonAccessDenied {
		t.Errorf("got reason=%v, want %s", resp["reason"], statusReasonAccessDenied)
	}
}

func TestStatus_PlainOutput_Authenticated(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "valid-token")

	out := runStatus(t, testutil.PlainOutput())
	if strings.TrimSuffix(out, "\n") != "true\tJane\tjane@example.com\t" {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestStatus_UsesEnvAccessToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer env-token" {
			t.Fatalf("got Authorization=%q, want Bearer env-token", got)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withEnvAccessToken(t, "env-token")

	out := runStatus(t)
	if !strings.Contains(out, "Source: "+config.EnvAccessToken) {
		t.Fatalf("expected env source in %q", out)
	}
}

func TestStatus_JSONOutput_UsesEnvAccessToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer env-token" {
			t.Fatalf("got Authorization=%q, want Bearer env-token", got)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane", "email": "jane@example.com"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withEnvAccessToken(t, "env-token")

	out := runStatus(t, testutil.JSONOutput())
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("status JSON output is invalid: %v\n%s", err, out)
	}
	if resp["source"] != string(config.TokenSourceEnv) {
		t.Fatalf("got source=%v, want %s", resp["source"], config.TokenSourceEnv)
	}
}

func TestStatus_EnvAccessTokenTakesPrecedenceOverConfig(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer env-token" {
			t.Fatalf("got Authorization=%q, want Bearer env-token", got)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "file-token")
	withEnvAccessToken(t, "env-token")

	runStatus(t)
}

func TestStatus_EnvAccessTokenIgnoresBrokenConfig(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer env-token" {
			t.Fatalf("got Authorization=%q, want Bearer env-token", got)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Jane"},
		}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	withEnvAccessToken(t, "env-token")

	runStatus(t)
}

func TestStatus_PlainOutput_NotLoggedIn(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API when not logged in")
	})

	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	out := runStatus(t, testutil.PlainOutput())
	if strings.TrimSuffix(out, "\n") != "false\t\t\t"+statusReasonNotLoggedIn {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestStatus_PlainOutput_InvalidToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "expired")

	out := runStatus(t, testutil.PlainOutput())
	if strings.TrimSuffix(out, "\n") != "false\t\t\t"+statusReasonInvalidOrExpired {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestStatus_PlainOutput_AccessDenied(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	withConfig(t, "restricted")

	out := runStatus(t, testutil.PlainOutput())
	if strings.TrimSuffix(out, "\n") != "false\t\t\t"+statusReasonAccessDenied {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

// --- Logout ---

func TestLogout_WithYes(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logout should not reach API")
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true))
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config was deleted
	_, statErr := os.Stat(filepath.Join(cfgDir, "gumroad", "config.json"))
	if statErr == nil {
		t.Error("config file should be deleted after logout")
	}
}

func TestLogout_RevokesAndDeletesStoredAdminToken(t *testing.T) {
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	if err := adminconfig.Save(&adminconfig.Config{Token: "admin-token"}); err != nil {
		t.Fatalf("Save admin config failed: %v", err)
	}
	adminPath, err := adminconfig.Path()
	if err != nil {
		t.Fatalf("adminconfig.Path: %v", err)
	}

	var revoked bool
	adminSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/admin/auth/revoke" {
			t.Fatalf("got path %q, want /internal/admin/auth/revoke", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
			t.Fatalf("got Authorization=%q, want Bearer admin-token", got)
		}
		revoked = true
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	}))
	t.Cleanup(adminSrv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, adminSrv.URL)

	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	if !revoked {
		t.Fatal("expected admin revoke request")
	}
	if _, err := os.Stat(adminPath); !os.IsNotExist(err) {
		t.Fatalf("admin config should be deleted, got err=%v", err)
	}
}

func TestLogout_KeepsAdminTokenWhenServerRevokeFails(t *testing.T) {
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	if err := adminconfig.Save(&adminconfig.Config{Token: "admin-token"}); err != nil {
		t.Fatalf("Save admin config failed: %v", err)
	}
	adminPath, err := adminconfig.Path()
	if err != nil {
		t.Fatalf("adminconfig.Path: %v", err)
	}
	adminSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "boom"}); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	}))
	t.Cleanup(adminSrv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, adminSrv.URL)

	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true))
	err = cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "couldn't revoke server-side") {
		t.Fatalf("expected revoke failure, got %v", err)
	}
	if _, statErr := os.Stat(adminPath); statErr != nil {
		t.Fatalf("admin config should remain after revoke failure: %v", statErr)
	}
}

func TestLogout_RequiresConfirmation(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})
	cmd := testutil.Command(newLogoutCmd(), testutil.NoInput(true))
	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error without confirmation")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestLogout_DryRunSkipsConfirmationAndPreservesConfig(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logout should not reach API")
	})
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "gumroad", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.DryRun(true), testutil.NoInput(true), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config should remain during dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "Dry run") || !strings.Contains(out.String(), "remove stored API token") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}
}

func TestLogout_ShowsMessage(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if !strings.Contains(out.String(), "Logged out") {
		t.Errorf("should show logout message: %q", out.String())
	}
}

func TestLogout_ShowsEnvAccessTokenNotice(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	withEnvAccessToken(t, "env-token")

	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true), testutil.Quiet(false), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if !strings.Contains(out.String(), config.EnvAccessToken) || !strings.Contains(out.String(), "still set") || !strings.Contains(out.String(), "gumroad auth status") {
		t.Fatalf("expected env token notice, got %q", out.String())
	}
}

func TestLogout_JSONOutput(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logout should not reach API")
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true), testutil.JSONOutput(), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("logout JSON output is invalid: %v\n%s", err, out.String())
	}
	if resp["authenticated"] != false {
		t.Fatalf("got authenticated=%v, want false", resp["authenticated"])
	}
	if resp["logged_out"] != true {
		t.Fatalf("got logged_out=%v, want true", resp["logged_out"])
	}
	if _, ok := resp["source"]; ok {
		t.Fatalf("unexpected source in logout JSON without env token: %v", resp["source"])
	}
}

func TestLogout_JSONOutput_EnvAccessTokenRemainsUnverified(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logout should not reach API")
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	withEnvAccessToken(t, "env-token")

	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true), testutil.JSONOutput(), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("logout JSON output is invalid: %v\n%s", err, out.String())
	}
	if resp["authenticated"] != false {
		t.Fatalf("got authenticated=%v, want false", resp["authenticated"])
	}
	if resp["source"] != string(config.TokenSourceEnv) {
		t.Fatalf("got source=%v, want %s", resp["source"], config.TokenSourceEnv)
	}
	if !strings.Contains(resp["message"].(string), config.EnvAccessToken) || !strings.Contains(resp["message"].(string), "still set") {
		t.Fatalf("expected message to mention %s, got %v", config.EnvAccessToken, resp["message"])
	}
}

func TestLogout_PlainOutput(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logout should not reach API")
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true), testutil.PlainOutput(), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	if got := strings.TrimRight(out.String(), "\n"); got != "false\ttrue\t" {
		t.Fatalf("unexpected plain output: %q", got)
	}
}

func TestLogout_PlainOutput_WithEnvToken(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("logout should not reach API")
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	withEnvAccessToken(t, "env-token")

	var out bytes.Buffer
	cmd := testutil.Command(newLogoutCmd(), testutil.Yes(true), testutil.PlainOutput(), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "false\ttrue\tenv" {
		t.Fatalf("unexpected plain output: %q, want %q", out.String(), "false\ttrue\tenv")
	}
}

func TestLogout_NonInteractiveRequiresYes(t *testing.T) {
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "gumroad"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gumroad", "config.json"), []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	cmd := testutil.Command(newLogoutCmd(), testutil.Stdin(strings.NewReader("n\n")))

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected non-interactive logout to require --yes")
	}
	if !strings.Contains(err.Error(), "stdin is not interactive") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Config should still exist
	if _, statErr := os.Stat(filepath.Join(cfgDir, "gumroad", "config.json")); statErr != nil {
		t.Error("config should not be deleted when confirmation is blocked")
	}
}

// --- OAuth Login ---

func withTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	old := isTerminalFunc
	isTerminalFunc = func(r interface{}) bool { return isTTY }
	t.Cleanup(func() { isTerminalFunc = old })
}

// withMockBrowser replaces oauth.OpenBrowser with a function that simulates
// the browser by hitting the callback endpoint directly.
func withMockBrowser(t *testing.T) {
	t.Helper()
	old := oauth.OpenBrowser
	oauth.OpenBrowser = func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", redirectURI, state)
		resp, err := http.Get(callbackURL) //nolint:gosec // G107: test-only, URL from test server
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
	t.Cleanup(func() { oauth.OpenBrowser = old })
}

// withMockBrowserFail replaces oauth.OpenBrowser with one that always fails.
func withMockBrowserFail(t *testing.T) {
	t.Helper()
	old := oauth.OpenBrowser
	oauth.OpenBrowser = func(authURL string) error {
		return fmt.Errorf("no display available")
	}
	t.Cleanup(func() { oauth.OpenBrowser = old })
}

// setupOAuthTokenServer creates a mock token endpoint and configures the oauth
// package to use it. Returns the API server for /user verification.
func setupOAuthTokenServer(t *testing.T) {
	t.Helper()
	setupOAuthTokenServerWithResponse(t, oauth.TokenResponse{
		AccessToken: "oauth-access-token-from-server",
		TokenType:   "bearer",
		Scope:       "edit_products view_sales",
	})
}

func setupOAuthTokenServerWithResponse(t *testing.T, tokenResponse oauth.TokenResponse) {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tokenResponse); err != nil {
			t.Errorf("encode token response: %v", err)
		}
	}))
	t.Cleanup(tokenSrv.Close)

	// Override OAuth constants for tests.
	oldCfg := oauth.DefaultFlowConfigFunc
	oauth.DefaultFlowConfigFunc = func() oauth.FlowConfig {
		cfg := oldCfg()
		cfg.TokenURL = tokenSrv.URL + "/oauth/token"
		return cfg
	}
	t.Cleanup(func() { oauth.DefaultFlowConfigFunc = oldCfg })
}

func TestLogin_OAuth_BrowserFlow(t *testing.T) {
	withTerminal(t, true)
	withMockBrowser(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer oauth-access-token-from-server" {
			w.WriteHeader(401)
			if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
				t.Errorf("encode response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "OAuth User", "email": "oauth@test.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.Quiet(false), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(out.String(), "OAuth User") {
		t.Errorf("expected user info in output: %q", out.String())
	}

	data, err := os.ReadFile(filepath.Join(cfgDir, "gumroad", "config.json"))
	if err != nil {
		t.Fatalf("config not saved: %v", err)
	}
	if !strings.Contains(string(data), "oauth-access-token-from-server") {
		t.Errorf("config should contain OAuth token, got: %s", data)
	}
}

func TestLogin_OAuth_BrowserFlowSavesAdminTokenFromSameApproval(t *testing.T) {
	withTerminal(t, true)
	withMockBrowser(t)
	setupOAuthTokenServerWithResponse(t, oauth.TokenResponse{
		AccessToken: "oauth-access-token-from-server",
		TokenType:   "bearer",
		Scope:       "edit_products view_sales",
		AdminToken: &oauth.AdminTokenResponse{
			Token:           "admin-token-from-oauth",
			TokenExternalID: "adm_123",
			Actor:           oauth.AdminActor{Name: "Admin User", Email: "admin@example.com"},
			ExpiresAt:       "2026-06-01T00:00:00Z",
		},
	})
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer oauth-access-token-from-server" {
			w.WriteHeader(401)
			if err := json.NewEncoder(w).Encode(map[string]any{"success": false}); err != nil {
				t.Errorf("encode response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "OAuth User", "email": "oauth@test.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	cmd := testutil.Command(newLoginCmd())
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	publicData, err := os.ReadFile(filepath.Join(cfgDir, "gumroad", "config.json"))
	if err != nil {
		t.Fatalf("public config not saved: %v", err)
	}
	if !strings.Contains(string(publicData), "oauth-access-token-from-server") {
		t.Fatalf("public config should contain seller OAuth token, got %s", publicData)
	}

	adminPath, err := adminconfig.Path()
	if err != nil {
		t.Fatalf("adminconfig.Path: %v", err)
	}
	adminData, err := os.ReadFile(adminPath)
	if err != nil {
		t.Fatalf("admin config not saved: %v", err)
	}
	for _, want := range []string{"admin-token-from-oauth", "adm_123", "admin@example.com", "2026-06-01T00:00:00Z"} {
		if !strings.Contains(string(adminData), want) {
			t.Fatalf("admin config missing %q: %s", want, adminData)
		}
	}
}

func TestLogin_OAuth_BrowserFails_FallsBackToHeadless(t *testing.T) {
	withTerminal(t, true)
	withMockBrowserFail(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Headless User", "email": "h@test.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer

	// Use a mutex-protected buffer for stderr to avoid data races
	// between the command goroutine writing and the test goroutine reading.
	errOut := &syncBuffer{}

	pr, pw, _ := os.Pipe()
	cmd := testutil.Command(newLoginCmd(), testutil.Quiet(false), testutil.Stdout(&out), testutil.Stderr(errOut), testutil.Stdin(pr))

	done := make(chan error, 1)
	go func() {
		done <- cmd.RunE(cmd, []string{})
	}()

	// Poll stderr until the headless prompt appears.
	var state string
	for i := 0; i < 200; i++ {
		time.Sleep(10 * time.Millisecond)
		content := errOut.String()
		if strings.Contains(content, "Paste the full URL") {
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "http") {
					u, _ := url.Parse(line)
					state = u.Query().Get("state")
					break
				}
			}
			break
		}
	}

	if state == "" {
		pw.Close()
		<-done
		t.Fatalf("could not extract state from headless prompt. stderr: %q", errOut.String())
	}

	fmt.Fprintf(pw, "http://127.0.0.1/callback?code=headless-code&state=%s\n", state)
	pw.Close()

	if err := <-done; err != nil {
		t.Fatalf("RunE: %v", err)
	}
}

func TestLogin_OAuth_WebFlag_NoFallback(t *testing.T) {
	withTerminal(t, true)
	withMockBrowserFail(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API when --web fails")
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	cmd := testutil.Command(newLoginCmd())
	if err := cmd.Flags().Set("web", "true"); err != nil {
		t.Fatalf("set --web flag: %v", err)
	}
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "browser login failed") {
		t.Fatalf("expected browser login failed error, got: %v", err)
	}
}

func TestLogin_OAuth_VerifyAndSave_401(t *testing.T) {
	withTerminal(t, true)
	withMockBrowser(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Unauthorized"}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	cmd := testutil.Command(newLoginCmd())
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("expected invalid token error, got: %v", err)
	}
}

func TestLogin_OAuth_JSONOutput(t *testing.T) {
	withTerminal(t, true)
	withMockBrowser(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "JSON User", "email": "json@test.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.JSONOutput(), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if resp["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", resp["authenticated"])
	}
}

func TestLogin_OAuth_PlainOutput(t *testing.T) {
	withTerminal(t, true)
	withMockBrowser(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Plain", "email": "p@t.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.PlainOutput(), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if strings.TrimSpace(out.String()) != "true\tPlain\tp@t.com" {
		t.Fatalf("unexpected plain output: %q", out.String())
	}
}

func TestLogin_OAuth_QuietOutput(t *testing.T) {
	withTerminal(t, true)
	withMockBrowser(t)
	setupOAuthTokenServer(t)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Quiet", "email": "q@t.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var out bytes.Buffer
	cmd := testutil.Command(newLoginCmd(), testutil.Quiet(true), testutil.Stdout(&out))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if out.String() != "" {
		t.Errorf("quiet mode should produce no output, got: %q", out.String())
	}
}

func TestLogin_PipedStdin_StillWorks(t *testing.T) {
	withTerminal(t, false)
	setupAuth(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"name": "Piped", "email": "piped@test.com"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	cmd := testutil.Command(newLoginCmd(), testutil.Stdin(strings.NewReader("piped-token\n")))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
}

func TestLogin_WebFlag(t *testing.T) {
	cmd := newLoginCmd()
	f := cmd.Flags().Lookup("web")
	if f == nil {
		t.Fatal("--web flag not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--web default = %q, want false", f.DefValue)
	}
}

// --- Constructor ---

func TestNewAuthCmd(t *testing.T) {
	cmd := NewAuthCmd()
	if cmd.Use != "auth" {
		t.Errorf("got Use=%q, want auth", cmd.Use)
	}
	subs := make(map[string]bool)
	for _, c := range cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"login", "status", "logout"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}
