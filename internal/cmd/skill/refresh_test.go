package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func overrideConfigDir(t *testing.T, dir string) {
	t.Helper()
	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDir = orig })
}

func TestAutoRefresh_SkipsDevVersion(t *testing.T) {
	dir := t.TempDir()
	overrideConfigDir(t, dir)

	AutoRefresh("dev")

	sentinel := filepath.Join(dir, sentinelFile)
	if _, err := os.Stat(sentinel); err == nil {
		t.Error("expected no sentinel for dev version")
	}
}

func TestAutoRefresh_SkipsEmptyVersion(t *testing.T) {
	dir := t.TempDir()
	overrideConfigDir(t, dir)

	AutoRefresh("")

	sentinel := filepath.Join(dir, sentinelFile)
	if _, err := os.Stat(sentinel); err == nil {
		t.Error("expected no sentinel for empty version")
	}
}

func TestAutoRefresh_WritesSentinel(t *testing.T) {
	dir := t.TempDir()
	overrideConfigDir(t, dir)
	overrideHomeDir(t, dir)

	AutoRefresh("1.0.0")

	data, err := os.ReadFile(filepath.Join(dir, sentinelFile))
	if err != nil {
		t.Fatalf("expected sentinel file: %v", err)
	}
	if string(data) != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", string(data))
	}
}

func TestAutoRefresh_SkipsWhenVersionUnchanged(t *testing.T) {
	dir := t.TempDir()
	overrideConfigDir(t, dir)
	overrideHomeDir(t, dir)

	// Create a skill file to track writes
	skillPath := filepath.Join(dir, ".agents", skillRelPath)
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	// First run writes sentinel
	AutoRefresh("1.0.0")

	// Restore old content to detect if it gets overwritten
	if err := os.WriteFile(skillPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	// Second run with same version should skip
	AutoRefresh("1.0.0")
	data, _ := os.ReadFile(skillPath)
	if string(data) != "old" {
		t.Error("expected skill file to be unchanged on same version")
	}
}

func TestAutoRefresh_RefreshesOnVersionChange(t *testing.T) {
	dir := t.TempDir()
	overrideConfigDir(t, dir)
	overrideHomeDir(t, dir)

	// Create an existing skill file
	skillPath := filepath.Join(dir, ".agents", skillRelPath)
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("old content"), 0600); err != nil {
		t.Fatal(err)
	}

	// Write v1 sentinel
	if err := os.WriteFile(filepath.Join(dir, sentinelFile), []byte("1.0.0"), 0600); err != nil {
		t.Fatal(err)
	}

	AutoRefresh("2.0.0")

	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("could not read skill: %v", err)
	}
	if string(data) == "old content" {
		t.Error("expected skill to be updated, still has old content")
	}
	if !strings.Contains(string(data), "gumroad") {
		t.Error("expected refreshed skill content")
	}

	sentinel, _ := os.ReadFile(filepath.Join(dir, sentinelFile))
	if string(sentinel) != "2.0.0" {
		t.Errorf("expected sentinel 2.0.0, got %q", string(sentinel))
	}
}

func TestAutoRefresh_OnlyRefreshesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	overrideConfigDir(t, dir)
	overrideHomeDir(t, dir)

	// Don't create any skill files — nothing should be created
	AutoRefresh("1.0.0")

	for _, d := range []string{".agents", ".claude", ".codex", ".opencode"} {
		p := filepath.Join(dir, d, skillRelPath)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("expected no file at %s (should not create new installs)", p)
		}
	}
}

func TestAutoRefresh_RefreshesSymlinkedFiles(t *testing.T) {
	if !symlinkSupported() {
		t.Skip("symlinks not supported")
	}

	dir := t.TempDir()
	overrideConfigDir(t, dir)
	overrideHomeDir(t, dir)

	// Create the baseline file
	basePath := filepath.Join(dir, ".agents", skillRelPath)
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(basePath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create a symlink from .claude to the baseline (directory-level)
	claudeSkillDir := filepath.Join(dir, ".claude", skillDirName)
	if err := os.MkdirAll(filepath.Dir(claudeSkillDir), 0755); err != nil {
		t.Fatal(err)
	}
	agentsSkillDir := filepath.Join(dir, ".agents", skillDirName)
	if err := os.Symlink(agentsSkillDir, claudeSkillDir); err != nil {
		t.Fatal(err)
	}

	AutoRefresh("1.0.0")

	// The baseline file should be refreshed (os.Stat follows symlinks)
	data, _ := os.ReadFile(basePath)
	if string(data) == "old" {
		t.Error("expected baseline file to be refreshed")
	}
}

func symlinkSupported() bool {
	dir, err := os.MkdirTemp("", "symlink-test")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		return false
	}
	return os.Symlink(target, filepath.Join(dir, "link")) == nil
}
