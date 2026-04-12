package skill

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func rootWithSkill() *cobra.Command {
	root := &cobra.Command{Use: "gumroad"}
	root.AddCommand(NewSkillCmd())
	return root
}

func TestSkill_NonTTY_PrintsToStdout(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(false)
	defer output.ResetStdoutIsTerminalForTesting()

	var stdout bytes.Buffer
	cmd := testutil.Command(NewSkillCmd(), testutil.Stdout(&stdout))

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "name: gumroad-cli") {
		t.Errorf("expected skill content, got %q", got[:min(len(got), 200)])
	}
	if !strings.Contains(got, "gumroad products list") {
		t.Errorf("expected command examples in skill content")
	}
}

func TestSkill_NoInput_PrintsToStdout(t *testing.T) {
	var stdout bytes.Buffer
	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(true), testutil.Stdout(&stdout))

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "name: gumroad-cli") {
		t.Errorf("expected skill content with --no-input, got %q", got[:min(len(got), 200)])
	}
}

func TestSkillInstall_CustomPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom", "SKILL.md")

	cmd := testutil.Command(newInstallCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--path", path})

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("could not read installed file: %v", readErr)
	}
	if !strings.Contains(string(content), "name: gumroad-cli") {
		t.Error("installed file does not contain expected skill content")
	}
}

func TestSkillInstall_DefaultLocations(t *testing.T) {
	dir := t.TempDir()

	// Override defaultTargets to use temp dir
	origTargets := defaultTargets
	defaultTargets = func() []installTarget {
		return []installTarget{
			{"Test Agent", filepath.Join(dir, ".agents", skillRelPath)},
		}
	}
	t.Cleanup(func() { defaultTargets = origTargets })

	// Create a fake ~/.claude dir so symlink path triggers
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("could not create claude dir: %v", err)
	}

	// Override userHomeDir for the install command
	origHome := userHomeDir
	userHomeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userHomeDir = origHome })

	cmd := testutil.Command(newInstallCmd())

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify shared file was written
	sharedPath := filepath.Join(dir, ".agents", skillRelPath)
	if _, statErr := os.Stat(sharedPath); statErr != nil {
		t.Errorf("expected shared file at %s", sharedPath)
	}

	// Verify symlink was created
	claudeSkillPath := filepath.Join(dir, ".claude", skillRelPath)
	info, statErr := os.Lstat(claudeSkillPath)
	if statErr != nil {
		t.Errorf("expected symlink at %s", claudeSkillPath)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink at %s, got regular file", claudeSkillPath)
	}

	// Verify symlink target resolves to correct content
	content, readErr := os.ReadFile(claudeSkillPath)
	if readErr != nil {
		t.Fatalf("could not read through symlink: %v", readErr)
	}
	if !strings.Contains(string(content), "name: gumroad-cli") {
		t.Error("symlinked file does not contain expected skill content")
	}
}

func TestSkillInstall_HomeError(t *testing.T) {
	origHome := userHomeDir
	userHomeDir = func() (string, error) { return "", fmt.Errorf("no home") }
	t.Cleanup(func() { userHomeDir = origHome })

	cmd := testutil.Command(newInstallCmd())
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "could not determine home directory") {
		t.Fatalf("expected home dir error, got %v", err)
	}
}

func TestSkillInstall_NoClaudeDir(t *testing.T) {
	dir := t.TempDir()
	// Don't create .claude dir — symlink step should be skipped

	origHome := userHomeDir
	userHomeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userHomeDir = origHome })

	cmd := testutil.Command(newInstallCmd())
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Shared file should exist
	sharedPath := filepath.Join(dir, ".agents", skillRelPath)
	if _, statErr := os.Stat(sharedPath); statErr != nil {
		t.Errorf("expected shared file at %s", sharedPath)
	}

	// Claude symlink should NOT exist
	claudeSkillPath := filepath.Join(dir, ".claude", skillRelPath)
	if _, statErr := os.Stat(claudeSkillPath); statErr == nil {
		t.Errorf("did not expect file at %s when ~/.claude doesn't exist", claudeSkillPath)
	}
}

func TestSkillInstall_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")

	// Write old content
	if err := os.WriteFile(path, []byte("old content"), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := testutil.Command(newInstallCmd())
	cmd.SetArgs([]string{"--path", path})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "old content") {
		t.Error("expected old content to be overwritten")
	}
	if !strings.Contains(string(content), "name: gumroad-cli") {
		t.Error("expected new skill content")
	}
}

func TestSkillInstall_Quiet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")

	cmd := testutil.Command(newInstallCmd(), testutil.Quiet(true))
	cmd.SetArgs([]string{"--path", path})

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stderr.Len() > 0 {
		t.Errorf("expected no stderr with --quiet, got %q", stderr.String())
	}
}

func TestSkill_TTY_SelectInstallTarget(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()

	dir := t.TempDir()
	installPath := filepath.Join(dir, "SKILL.md")

	origTargets := defaultTargets
	defaultTargets = func() []installTarget {
		return []installTarget{
			{"Test", installPath},
		}
	}
	t.Cleanup(func() { defaultTargets = origTargets })

	origSelect := selectFunc
	selectFunc = func(msg string, opts []prompt.SelectOption, in io.Reader, out io.Writer, noInput bool) (string, error) {
		// Simulate choosing the first option (the install path)
		return opts[0].Value, nil
	}
	t.Cleanup(func() { selectFunc = origSelect })

	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(false))
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, readErr := os.ReadFile(installPath)
	if readErr != nil {
		t.Fatalf("expected file at %s: %v", installPath, readErr)
	}
	if !strings.Contains(string(content), "name: gumroad-cli") {
		t.Error("installed file missing expected content")
	}
}

func TestSkill_TTY_SelectStdout(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()

	origTargets := defaultTargets
	defaultTargets = func() []installTarget {
		return []installTarget{{"Test", "/tmp/unused"}}
	}
	t.Cleanup(func() { defaultTargets = origTargets })

	origSelect := selectFunc
	selectFunc = func(msg string, opts []prompt.SelectOption, in io.Reader, out io.Writer, noInput bool) (string, error) {
		return selectValPrint, nil
	}
	t.Cleanup(func() { selectFunc = origSelect })

	var stdout bytes.Buffer
	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(false), testutil.Stdout(&stdout))
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "name: gumroad-cli") {
		t.Error("expected skill content on stdout")
	}
}

func TestSkill_TTY_SelectError(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()

	origTargets := defaultTargets
	defaultTargets = func() []installTarget {
		return []installTarget{{"Test", "/tmp/unused"}}
	}
	t.Cleanup(func() { defaultTargets = origTargets })

	origSelect := selectFunc
	selectFunc = func(msg string, opts []prompt.SelectOption, in io.Reader, out io.Writer, noInput bool) (string, error) {
		return "", fmt.Errorf("user cancelled")
	}
	t.Cleanup(func() { selectFunc = origSelect })

	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(false))
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "user cancelled") {
		t.Fatalf("expected 'user cancelled' error, got %v", err)
	}
}

func TestDefaultTargets_NoHome(t *testing.T) {
	origHome := userHomeDir
	userHomeDir = func() (string, error) { return "", fmt.Errorf("no home") }
	t.Cleanup(func() { userHomeDir = origHome })

	targets := defaultTargets()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target without HOME, got %d", len(targets))
	}
	if targets[0].Label != "Claude Code (Project)" {
		t.Errorf("expected project target, got %q", targets[0].Label)
	}
}

func TestDefaultTargets_WithHome(t *testing.T) {
	origHome := userHomeDir
	userHomeDir = func() (string, error) { return "/fakehome", nil }
	t.Cleanup(func() { userHomeDir = origHome })

	targets := defaultTargets()
	if len(targets) != 5 {
		t.Fatalf("expected 5 targets with HOME, got %d", len(targets))
	}
}

func TestSkillInstall_ReplacesExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	linkPath := filepath.Join(dir, "link")
	target1 := filepath.Join(dir, "target1")
	target2 := filepath.Join(dir, "target2")

	if err := os.WriteFile(target1, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target2, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target1, linkPath); err != nil {
		t.Fatal(err)
	}

	opts := testutil.TestOptions()
	err := symlinkSkillFile(linkPath, target2, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved, _ := os.Readlink(linkPath)
	if resolved != target2 {
		t.Errorf("expected symlink to %s, got %s", target2, resolved)
	}
}

func TestSymlinkSkillFile_InvalidDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	// Use a file as parent to make MkdirAll fail (cross-platform)
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	opts := testutil.TestOptions()
	err := symlinkSkillFile(filepath.Join(blocker, "sub", "link"), filepath.Join(dir, "target"), opts)
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
	if !strings.Contains(err.Error(), "could not create directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSymlinkSkillFile_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target")
	if err := os.WriteFile(targetPath, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(dir, "sub", "link")
	opts := testutil.TestOptions()
	err := symlinkSkillFile(linkPath, targetPath, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved, _ := os.Readlink(linkPath)
	if resolved != targetPath {
		t.Errorf("expected symlink to %s, got %s", targetPath, resolved)
	}
}

func TestSymlinkSkillFile_NoExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target")
	if err := os.WriteFile(targetPath, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(dir, "newlink")
	opts := testutil.TestOptions()
	err := symlinkSkillFile(linkPath, targetPath, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved, _ := os.Readlink(linkPath)
	if resolved != targetPath {
		t.Errorf("expected symlink to %s, got %s", targetPath, resolved)
	}
}

func TestWriteSkillFile_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	// Use a file as parent to make MkdirAll fail (cross-platform)
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	opts := testutil.TestOptions()
	err := writeSkillFile(filepath.Join(blocker, "sub", "SKILL.md"), []byte("content"), opts)
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestWriteSkillFile_ReadOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permissions")
	}

	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0755) })

	opts := testutil.TestOptions()
	err := writeSkillFile(filepath.Join(readOnlyDir, "sub", "SKILL.md"), []byte("content"), opts)
	if err == nil {
		t.Fatal("expected error for read-only directory")
	}
}

func TestWriteSkillFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "SKILL.md")
	opts := testutil.TestOptions()
	err := writeSkillFile(path, []byte("skill content"), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "skill content" {
		t.Errorf("got %q", string(data))
	}
}

func TestSkill_EmbedError(t *testing.T) {
	origRead := readSkill
	readSkill = func() ([]byte, error) { return nil, fmt.Errorf("embed broken") }
	t.Cleanup(func() { readSkill = origRead })

	var stdout bytes.Buffer
	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(true), testutil.Stdout(&stdout))
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "could not read embedded skill") {
		t.Fatalf("expected embed error, got %v", err)
	}
}

func TestSkillInstall_EmbedError(t *testing.T) {
	origRead := readSkill
	readSkill = func() ([]byte, error) { return nil, fmt.Errorf("embed broken") }
	t.Cleanup(func() { readSkill = origRead })

	cmd := testutil.Command(newInstallCmd())
	cmd.SetArgs([]string{"--path", "/tmp/test"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not read embedded skill") {
		t.Fatalf("expected embed error, got %v", err)
	}
}

func TestSkill_NoArgs(t *testing.T) {
	root := rootWithSkill()
	root.SetArgs([]string{"skill", "bogus"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for extra arg")
	}
}

func TestSkill_Help(t *testing.T) {
	root := rootWithSkill()
	root.SetArgs([]string{"skill", "--help"})

	var stdout bytes.Buffer
	root.SetOut(&stdout)

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "Print or install") {
		t.Errorf("expected help text, got %q", got[:min(len(got), 200)])
	}
}

func TestSkillInstall_Help(t *testing.T) {
	root := rootWithSkill()
	root.SetArgs([]string{"skill", "install", "--help"})

	var stdout bytes.Buffer
	root.SetOut(&stdout)

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "--path") {
		t.Errorf("expected --path flag in help")
	}
}
