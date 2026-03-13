package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

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
	if strings.TrimSpace(out.String()) != "false\ttrue" {
		t.Fatalf("unexpected plain output: %q", out.String())
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
