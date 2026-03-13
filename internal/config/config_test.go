package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := &Config{AccessToken: "test-token-abc"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.AccessToken != "test-token-abc" {
		t.Errorf("got token %q, want %q", loaded.AccessToken, "test-token-abc")
	}
}

func TestSave_ReplacesExistingConfigWithoutTempFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Save(&Config{AccessToken: "first-token"}); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}
	if err := Save(&Config{AccessToken: "second-token"}); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.AccessToken != "second-token" {
		t.Fatalf("got token %q, want %q", loaded.AccessToken, "second-token")
	}

	entries, err := os.ReadDir(filepath.Join(tmp, "gumroad"))
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("unexpected files in config dir: %v", entries)
	}
}

func TestReplaceFile_WindowsReplacesExistingDestination(t *testing.T) {
	oldGoos := goos
	goos = "windows"
	t.Cleanup(func() { goos = oldGoos })

	dir := t.TempDir()
	dst := filepath.Join(dir, "config.json")
	tmp := filepath.Join(dir, "config.json.tmp")

	if err := os.WriteFile(dst, []byte(`{"access_token":"old"}`), 0600); err != nil {
		t.Fatalf("WriteFile dst failed: %v", err)
	}
	if err := os.WriteFile(tmp, []byte(`{"access_token":"new"}`), 0600); err != nil {
		t.Fatalf("WriteFile tmp failed: %v", err)
	}

	if err := replaceFile(tmp, dst); err != nil {
		t.Fatalf("replaceFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != `{"access_token":"new"}` {
		t.Fatalf("got %q, want new contents", string(data))
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed, got err=%v", err)
	}
}

func TestReplaceFile_NonWindowsRenamesIntoPlace(t *testing.T) {
	oldGoos := goos
	goos = "darwin"
	t.Cleanup(func() { goos = oldGoos })

	dir := t.TempDir()
	dst := filepath.Join(dir, "config.json")
	tmp := filepath.Join(dir, "config.json.tmp")

	if err := os.WriteFile(tmp, []byte(`{"access_token":"new"}`), 0600); err != nil {
		t.Fatalf("WriteFile tmp failed: %v", err)
	}

	if err := replaceFile(tmp, dst); err != nil {
		t.Fatalf("replaceFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != `{"access_token":"new"}` {
		t.Fatalf("got %q, want new contents", string(data))
	}
}

func TestReplaceFile_WindowsRemoveError(t *testing.T) {
	oldGoos := goos
	goos = "windows"
	t.Cleanup(func() { goos = oldGoos })

	dir := t.TempDir()
	dst := filepath.Join(dir, "config-dir")
	tmp := filepath.Join(dir, "config.json.tmp")

	if err := os.MkdirAll(filepath.Join(dst, "child"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(tmp, []byte(`{"access_token":"new"}`), 0600); err != nil {
		t.Fatalf("WriteFile tmp failed: %v", err)
	}

	if err := replaceFile(tmp, dst); err == nil {
		t.Fatal("expected replaceFile to fail when os.Remove(path) fails")
	}
}

func TestWriteConfigAtomically_CreateTempError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	path := filepath.Join(dir, "config.json")

	err := writeConfigAtomically(path, []byte(`{"access_token":"tok"}`))
	if err == nil {
		t.Fatal("expected error when config directory does not exist")
	}
}

func TestWriteConfigAtomically_CleansUpTempFileOnReplaceError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config-dir")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	err := writeConfigAtomically(path, []byte(`{"access_token":"tok"}`))
	if err == nil {
		t.Fatal("expected replace error when destination is a directory")
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir failed: %v", readErr)
	}
	if len(entries) != 1 || entries[0].Name() != "config-dir" {
		t.Fatalf("expected temp file cleanup, got entries %v", entries)
	}
}

func TestFilePermissions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Save(&Config{AccessToken: "tok"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(tmp, "gumroad", "config.json"))
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("got permissions %o, want 0600", perm)
	}
}

func TestLoad_InsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission checks do not apply on Windows")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "gumroad")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected insecure-permissions error")
	}
	if !strings.Contains(err.Error(), "insecure permissions 0644") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("expected chmod guidance, got: %v", err)
	}
}

func TestLoadMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.AccessToken != "" {
		t.Errorf("expected empty token from missing config, got %q", cfg.AccessToken)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "gumroad")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("not json{{{"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON config")
	}
	if !strings.Contains(err.Error(), "could not parse config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_Unreadable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "gumroad")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Chmod(p, 0000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(p, 0600); err != nil {
			t.Errorf("cleanup chmod failed: %v", err)
		}
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unreadable config")
	}
	if !strings.Contains(err.Error(), "could not read config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTokenEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := Token()
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), EnvAccessToken) {
		t.Errorf("error should mention %s: %v", EnvAccessToken, err)
	}
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("expected errors.Is(err, ErrNotAuthenticated), got %v", err)
	}
}

func TestToken_Valid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Save(&Config{AccessToken: "my-token"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	tok, err := Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "my-token" {
		t.Errorf("got %q, want my-token", tok)
	}
}

func TestToken_UsesEnvAccessToken(t *testing.T) {
	t.Setenv(EnvAccessToken, "env-token")

	tok, err := Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "env-token" {
		t.Fatalf("got %q, want env-token", tok)
	}
}

func TestToken_EnvAccessTokenTakesPrecedence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(EnvAccessToken, "env-token")

	if err := Save(&Config{AccessToken: "file-token"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := ResolveToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Value != "env-token" {
		t.Fatalf("got token %q, want env-token", info.Value)
	}
	if info.Source != TokenSourceEnv {
		t.Fatalf("got source %q, want %q", info.Source, TokenSourceEnv)
	}
}

func TestToken_EnvAccessTokenIgnoresBrokenConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(EnvAccessToken, "env-token")

	dir := filepath.Join(tmp, "gumroad")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	tok, err := Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "env-token" {
		t.Fatalf("got %q, want env-token", tok)
	}
}

func TestDeleteConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Save(&Config{AccessToken: "tok"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.AccessToken != "" {
		t.Errorf("expected empty token after delete, got %q", cfg.AccessToken)
	}
}

func TestDelete_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Should not error when file doesn't exist
	if err := Delete(); err != nil {
		t.Fatalf("Delete of non-existent config should not error: %v", err)
	}
}

func TestDirXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir failed: %v", err)
	}
	if dir != "/tmp/test-xdg/gumroad" {
		t.Errorf("got dir %q, want %q", dir, "/tmp/test-xdg/gumroad")
	}
}

func TestDir_WindowsAppData(t *testing.T) {
	oldGoos := goos
	goos = "windows"
	t.Cleanup(func() { goos = oldGoos })

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", `C:\Users\test\AppData\Roaming`)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir failed: %v", err)
	}
	want := filepath.Join(`C:\Users\test\AppData\Roaming`, "gumroad")
	if dir != want {
		t.Errorf("got dir %q, want %q", dir, want)
	}
}

func TestDir_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	oldGoos := goos
	goos = "darwin"
	t.Cleanup(func() { goos = oldGoos })

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir failed: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "gumroad")
	if dir != want {
		t.Errorf("got dir %q, want %q", dir, want)
	}
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-path")
	p, err := Path()
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}
	want := "/tmp/test-path/gumroad/config.json"
	if p != want {
		t.Errorf("got %q, want %q", p, want)
	}
}

func TestDirPermissions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Save(&Config{AccessToken: "tok"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(tmp, "gumroad"))
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("got dir permissions %o, want 0700", perm)
	}
}

func TestSave_UnwritableDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "gumroad")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Errorf("cleanup chmod failed: %v", err)
		}
	})

	err := Save(&Config{AccessToken: "tok"})
	if err == nil {
		t.Fatal("expected error writing to unwritable file")
	}
	if !strings.Contains(err.Error(), "could not write config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSave_MkdirAllError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Create a file where the directory should be, blocking MkdirAll
	blocker := filepath.Join(tmp, "gumroad")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(blocker); err != nil && !os.IsNotExist(err) {
			t.Errorf("cleanup remove failed: %v", err)
		}
	})

	err := Save(&Config{AccessToken: "tok"})
	if err == nil {
		t.Fatal("expected error from MkdirAll")
	}
	if !strings.Contains(err.Error(), "could not create config directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func withBrokenHomeDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	orig := userHomeDir
	userHomeDir = func() (string, error) {
		return "", fmt.Errorf("no home directory")
	}
	t.Cleanup(func() { userHomeDir = orig })
}

func TestDir_HomeDirError(t *testing.T) {
	withBrokenHomeDir(t)
	_, err := Dir()
	if err == nil {
		t.Fatal("expected error when UserHomeDir fails")
	}
	if !strings.Contains(err.Error(), "could not determine home directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPath_DirError(t *testing.T) {
	withBrokenHomeDir(t)
	_, err := Path()
	if err == nil {
		t.Fatal("expected error when Dir fails")
	}
}

func TestLoad_PathError(t *testing.T) {
	withBrokenHomeDir(t)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Path fails")
	}
}

func TestSave_PathError(t *testing.T) {
	withBrokenHomeDir(t)
	err := Save(&Config{AccessToken: "tok"})
	if err == nil {
		t.Fatal("expected error when Path fails")
	}
}

func TestDelete_PathError(t *testing.T) {
	withBrokenHomeDir(t)
	err := Delete()
	if err == nil {
		t.Fatal("expected error when Path fails")
	}
}

func TestToken_LoadError(t *testing.T) {
	withBrokenHomeDir(t)
	_, err := Token()
	if err == nil {
		t.Fatal("expected error when Load fails")
	}
}

func TestDelete_PermissionDenied(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "gumroad")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Make parent directory read-only so Remove fails with permission error
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Errorf("cleanup chmod failed: %v", err)
		}
	})

	err := Delete()
	if err == nil {
		t.Fatal("expected permission error from Delete")
	}
	if !strings.Contains(err.Error(), "could not delete config") {
		t.Errorf("unexpected error: %v", err)
	}
}
