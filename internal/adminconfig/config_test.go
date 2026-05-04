package adminconfig

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/config"
)

func TestSaveLoadAndDelete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Save(&Config{
		Token:           "admin-token",
		TokenExternalID: "adm_123",
		Actor:           Actor{Name: "Admin User", Email: "admin@example.com"},
		ExpiresAt:       "2026-06-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Token != "admin-token" {
		t.Fatalf("got token %q, want admin-token", loaded.Token)
	}
	if loaded.TokenExternalID != "adm_123" || loaded.Actor.Email != "admin@example.com" || loaded.ExpiresAt == "" {
		t.Fatalf("admin metadata was not preserved: %+v", loaded)
	}

	if err := Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	loaded, err = Load()
	if err != nil {
		t.Fatalf("Load after Delete failed: %v", err)
	}
	if loaded.Token != "" {
		t.Fatalf("expected empty token after Delete, got %q", loaded.Token)
	}
}

func TestPathUsesSeparateAdminConfigFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	adminPath, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}
	publicPath, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path failed: %v", err)
	}

	if adminPath == publicPath {
		t.Fatalf("admin config should not share public config path %q", adminPath)
	}
	if filepath.Base(adminPath) != "admin.token" {
		t.Fatalf("got admin path %q, want admin.token file", adminPath)
	}
}

func TestLoadIgnoresLegacyAdminJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir failed: %v", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin.json"), []byte(`{"access_token":"old-admin-token"}`), 0600); err != nil {
		t.Fatalf("WriteFile legacy failed: %v", err)
	}

	_, err = ResolveStoredToken()
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("got error %v, want ErrNotAuthenticated", err)
	}
}

func TestTokenIgnoresLegacyAccessTokenField(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir failed: %v", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"access_token":"old-admin-token"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = ResolveStoredToken()
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("got error %v, want ErrNotAuthenticated", err)
	}
}

func TestTokenUsesEnvAccessToken(t *testing.T) {
	t.Setenv(EnvAccessToken, "env-admin-token")

	info, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken failed: %v", err)
	}
	if info.Value != "env-admin-token" {
		t.Fatalf("got token %q, want env-admin-token", info.Value)
	}
	if info.Source != TokenSourceEnv {
		t.Fatalf("got source %q, want %q", info.Source, TokenSourceEnv)
	}
}

func TestTokenIgnoresLegacyEnvAccessToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvAccessToken, "")
	t.Setenv("GUMROAD_ADMIN_ACCESS_TOKEN", "legacy-admin-token")

	_, err := ResolveToken()
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("got error %v, want ErrNotAuthenticated", err)
	}
}

func TestTokenEnvTakesPrecedenceOverStoredConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(EnvAccessToken, "env-admin-token")

	if err := Save(&Config{Token: "file-admin-token"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken failed: %v", err)
	}
	if info.Value != "env-admin-token" {
		t.Fatalf("got token %q, want env-admin-token", info.Value)
	}
	if info.Source != TokenSourceEnv {
		t.Fatalf("got source %q, want %q", info.Source, TokenSourceEnv)
	}
}

func TestTokenDoesNotUsePublicConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(EnvAccessToken, "")

	if err := config.Save(&config.Config{AccessToken: "public-token"}); err != nil {
		t.Fatalf("public config Save failed: %v", err)
	}

	_, err := Token()
	if err == nil {
		t.Fatal("expected missing admin token error")
	}
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("expected ErrNotAuthenticated, got %v", err)
	}
	if !strings.Contains(err.Error(), "check the admin box") {
		t.Fatalf("expected error to mention admin login, got %v", err)
	}
}

func TestLoadInsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission checks do not apply on Windows")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir failed: %v", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"token":"tok"}`), 0644); err != nil { //nolint:gosec // G306: intentionally insecure permissions for validation test.
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = Load()
	if err == nil {
		t.Fatal("expected insecure-permissions error")
	}
	if !strings.Contains(err.Error(), "insecure permissions 0644") {
		t.Fatalf("unexpected error: %v", err)
	}
}
