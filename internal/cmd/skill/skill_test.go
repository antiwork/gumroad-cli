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

func setInteractive(t *testing.T, interactive bool) {
	t.Helper()
	orig := prompt.IsInteractive
	prompt.IsInteractive = func(io.Reader) bool { return interactive }
	t.Cleanup(func() { prompt.IsInteractive = orig })
}

func overrideHomeDir(t *testing.T, dir string) {
	t.Helper()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userHomeDir = orig })
}

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
	if !strings.Contains(got, "name: gumroad") {
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
	if !strings.Contains(got, "name: gumroad") {
		t.Errorf("expected skill content with --no-input, got %q", got[:min(len(got), 200)])
	}
}

func TestSkill_PipedStdin_FallsBackToStdout(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	var stdout bytes.Buffer
	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(false), testutil.Stdout(&stdout), testutil.Stdin(r))

	runErr := cmd.RunE(cmd, []string{})
	r.Close()
	if runErr != nil {
		t.Fatalf("expected fallback to stdout, got error: %v", runErr)
	}

	if !strings.Contains(stdout.String(), "name: gumroad") {
		t.Error("expected skill content on stdout when stdin is piped")
	}
}

func TestSkillInstall_CustomPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom", "SKILL.md")

	cmd := testutil.Command(newInstallCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--path", path})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("could not read installed file: %v", readErr)
	}
	if !strings.Contains(string(content), "name: gumroad") {
		t.Error("installed file does not contain expected skill content")
	}
}

func TestSkillInstall_DefaultLocations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	overrideHomeDir(t, dir)

	// Create ~/.claude so symlink triggers
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := testutil.Command(newInstallCmd())
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify shared file
	sharedFile := filepath.Join(dir, ".agents", skillRelPath)
	if _, statErr := os.Stat(sharedFile); statErr != nil {
		t.Errorf("expected shared file at %s", sharedFile)
	}

	// Verify directory symlink at ~/.claude/skills/gumroad
	claudeSkillDir := filepath.Join(dir, ".claude", skillDirName)
	info, statErr := os.Lstat(claudeSkillDir)
	if statErr != nil {
		t.Fatalf("expected symlink dir at %s: %v", claudeSkillDir, statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected directory symlink at %s, got mode %v", claudeSkillDir, info.Mode())
	}

	// Verify symlink target is relative
	target, _ := os.Readlink(claudeSkillDir)
	if !strings.HasPrefix(target, "..") {
		t.Errorf("expected relative symlink target, got %q", target)
	}

	// Verify content resolves through symlink
	content, readErr := os.ReadFile(filepath.Join(claudeSkillDir, skillFileName))
	if readErr != nil {
		t.Fatalf("could not read through symlink: %v", readErr)
	}
	if !strings.Contains(string(content), "name: gumroad") {
		t.Error("symlinked file does not contain expected content")
	}
}

func TestSkillInstall_FallbackCopyOnSymlinkFailure(t *testing.T) {
	dir := t.TempDir()
	overrideHomeDir(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write shared baseline first
	content, _ := readSkill()
	sharedDir := filepath.Join(dir, ".agents", skillDirName)
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, skillFileName), content, 0644); err != nil { //nolint:gosec // G306: test file
		t.Fatal(err)
	}

	// Pre-create a regular file at the symlink target to block symlink creation
	claudeSkillDir := filepath.Join(dir, ".claude", skillDirName)
	if err := os.MkdirAll(claudeSkillDir, 0755); err != nil {
		t.Fatal(err)
	}

	// linkOrCopySkillDir uses RemoveAll first, so it will remove the dir and try symlink.
	// To actually test the copy fallback, we need symlink to fail.
	// This is hard to simulate portably. Instead, test copySkillDir directly.
	dstDir := filepath.Join(dir, "copy-target")
	opts := testutil.TestOptions()
	err := copySkillDir(sharedDir, dstDir, opts)
	if err != nil {
		t.Fatalf("copySkillDir failed: %v", err)
	}

	copied, readErr := os.ReadFile(filepath.Join(dstDir, skillFileName))
	if readErr != nil {
		t.Fatalf("could not read copied file: %v", readErr)
	}
	if !strings.Contains(string(copied), "name: gumroad") {
		t.Error("copied file missing expected content")
	}
}

func TestSkillInstall_HomeError(t *testing.T) {
	overrideHomeDir(t, "")
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
	overrideHomeDir(t, dir)

	cmd := testutil.Command(newInstallCmd())
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sharedPath := filepath.Join(dir, ".agents", skillRelPath)
	if _, statErr := os.Stat(sharedPath); statErr != nil {
		t.Errorf("expected shared file at %s", sharedPath)
	}

	claudeSkillDir := filepath.Join(dir, ".claude", skillDirName)
	if _, statErr := os.Stat(claudeSkillDir); statErr == nil {
		t.Errorf("did not expect dir at %s when ~/.claude doesn't exist", claudeSkillDir)
	}
}

func TestSkillInstall_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")

	if err := os.WriteFile(path, []byte("old content"), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := testutil.Command(newInstallCmd())
	cmd.SetArgs([]string{"--path", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "old content") {
		t.Error("expected old content to be overwritten")
	}
	if !strings.Contains(string(content), "name: gumroad") {
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

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stderr.Len() > 0 {
		t.Errorf("expected no stderr with --quiet, got %q", stderr.String())
	}
}

func TestSkill_TTY_SelectInstallTarget(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()
	setInteractive(t, true)

	dir := t.TempDir()
	installPath := filepath.Join(dir, "SKILL.md")

	origSelect := selectFunc
	selectFunc = func(msg string, opts []prompt.SelectOption, in io.Reader, out io.Writer, noInput bool) (string, error) {
		return installPath, nil
	}
	t.Cleanup(func() { selectFunc = origSelect })

	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(false))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, readErr := os.ReadFile(installPath)
	if readErr != nil {
		t.Fatalf("expected file at %s: %v", installPath, readErr)
	}
	if !strings.Contains(string(content), "name: gumroad") {
		t.Error("installed file missing expected content")
	}
}

func TestSkill_TTY_SelectStdout(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()
	setInteractive(t, true)

	origSelect := selectFunc
	selectFunc = func(msg string, opts []prompt.SelectOption, in io.Reader, out io.Writer, noInput bool) (string, error) {
		return selectValPrint, nil
	}
	t.Cleanup(func() { selectFunc = origSelect })

	var stdout bytes.Buffer
	cmd := testutil.Command(NewSkillCmd(), testutil.NoInput(false), testutil.Stdout(&stdout))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "name: gumroad") {
		t.Error("expected skill content on stdout")
	}
}

func TestSkill_TTY_SelectError(t *testing.T) {
	output.SetStdoutIsTerminalForTesting(true)
	defer output.ResetStdoutIsTerminalForTesting()
	setInteractive(t, true)

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

func TestExpandPath(t *testing.T) {
	overrideHomeDir(t, "/fakehome")

	tests := []struct {
		input string
		want  string
	}{
		{"~/.agents/skills", "/fakehome/.agents/skills"},
		{".claude/skills", ".claude/skills"},
		{"/absolute/path", "/absolute/path"},
	}
	for _, tt := range tests {
		got := expandPath(tt.input)
		if got != tt.want {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCodexGlobalSkillPath_Default(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	path := codexGlobalSkillPath()
	if !strings.Contains(path, ".codex") {
		t.Errorf("expected default codex path, got %q", path)
	}
}

func TestCodexGlobalSkillPath_CustomHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "/custom/codex")
	path := codexGlobalSkillPath()
	if !strings.HasPrefix(path, "/custom/codex") {
		t.Errorf("expected custom codex path, got %q", path)
	}
}

func TestLinkOrCopySkillDir_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("content"), 0644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}

	linkPath := filepath.Join(dir, "link")
	opts := testutil.TestOptions()
	if err := linkOrCopySkillDir(linkPath, srcDir, srcDir, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, _ := os.Lstat(linkPath)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink")
	}
}

func TestLinkOrCopySkillDir_ExistingDirFallsToCopy(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("skill"), 0644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}

	// Pre-create a real directory at the link path
	linkPath := filepath.Join(dir, "link")
	if err := os.MkdirAll(linkPath, 0755); err != nil {
		t.Fatal(err)
	}
	// Put a custom file the user might have
	if err := os.WriteFile(filepath.Join(linkPath, "custom.txt"), []byte("user data"), 0644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}

	opts := testutil.TestOptions()
	if err := linkOrCopySkillDir(linkPath, "../src", srcDir, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SKILL.md should be copied
	data, _ := os.ReadFile(filepath.Join(linkPath, "SKILL.md"))
	if string(data) != "skill" {
		t.Errorf("expected copied SKILL.md, got %q", string(data))
	}

	// User's custom file should be preserved
	custom, _ := os.ReadFile(filepath.Join(linkPath, "custom.txt"))
	if string(custom) != "user data" {
		t.Error("expected user's custom.txt to be preserved")
	}
}

func TestLinkOrCopySkillDir_ReplacesExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("new"), 0644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}

	// Create an old symlink pointing elsewhere
	linkPath := filepath.Join(dir, "link")
	oldTarget := filepath.Join(dir, "old")
	if err := os.MkdirAll(oldTarget, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oldTarget, linkPath); err != nil {
		t.Fatal(err)
	}

	opts := testutil.TestOptions()
	if err := linkOrCopySkillDir(linkPath, srcDir, srcDir, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a new symlink to srcDir
	target, _ := os.Readlink(linkPath)
	if target != srcDir {
		t.Errorf("expected symlink to %s, got %s", srcDir, target)
	}
}

func TestLinkOrCopySkillDir_InvalidParent(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	opts := testutil.TestOptions()
	err := linkOrCopySkillDir(filepath.Join(blocker, "sub", "link"), "../src", dir, opts)
	if err == nil {
		t.Fatal("expected error for invalid parent directory")
	}
}

func TestCopySkillDir_InvalidSrc(t *testing.T) {
	dir := t.TempDir()
	opts := testutil.TestOptions()
	err := copySkillDir(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "dst"), opts)
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestCopySkillDir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("skill"), 0644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}

	dstDir := filepath.Join(dir, "dst")
	opts := testutil.TestOptions()
	if err := copySkillDir(srcDir, dstDir, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dstDir, "SKILL.md"))
	if string(data) != "skill" {
		t.Errorf("got %q", string(data))
	}
}

func TestWriteSkillFile_InvalidPath(t *testing.T) {
	dir := t.TempDir()
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
	if err := writeSkillFile(path, []byte("skill content"), opts); err != nil {
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

	if err := root.Execute(); err != nil {
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

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "--path") {
		t.Errorf("expected --path flag in help")
	}
}
