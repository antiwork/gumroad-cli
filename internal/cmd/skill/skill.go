package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/antiwork/gumroad-cli/skills"
	"github.com/spf13/cobra"
)

const (
	skillDirName   = "skills/gumroad"
	skillFileName  = "SKILL.md"
	skillRelPath   = skillDirName + "/" + skillFileName
	selectValPrint = "" // sentinel: user chose "Print to stdout"
)

// skillLocation represents a predefined install target.
type skillLocation struct {
	Label string
	Path  string // may contain ~ prefix
}

var userHomeDir = os.UserHomeDir
var readSkill = skills.SkillMarkdown

func codexGlobalSkillPath() string {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, skillRelPath)
	}
	return "~/.codex/" + skillRelPath
}

// skillLocations is the canonical list of install targets.
// Used by the interactive menu, headless install, and auto-refresh.
var skillLocations = []skillLocation{
	{"Agents (Shared)", "~/.agents/" + skillRelPath},
	{"Claude Code (Global)", "~/.claude/" + skillRelPath},
	{"Claude Code (Project)", ".claude/" + skillRelPath},
	{"Codex (Global)", codexGlobalSkillPath()},
	{"OpenCode (Global)", "~/.opencode/" + skillRelPath},
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := userHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print or install the embedded AI agent skill",
		Long: `Print or install the embedded AI agent skill file (SKILL.md).

When piped or in non-TTY mode, prints the skill to stdout.
When interactive, shows a menu to choose an install location.`,
		Example: `  # Print skill to stdout (for piping)
  gumroad skill > SKILL.md

  # Interactive install (shows menu)
  gumroad skill

  # Headless install to default locations
  gumroad skill install`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return runSkill(opts)
		},
	}

	cmd.AddCommand(newInstallCmd())
	return cmd
}

var selectFunc = prompt.Select

func runSkill(opts cmdutil.Options) error {
	content, err := readSkill()
	if err != nil {
		return fmt.Errorf("could not read embedded skill: %w", err)
	}

	if !output.IsTTY() || opts.NoInput || !prompt.IsInteractive(opts.In()) {
		_, err := opts.Out().Write(content)
		return err
	}

	selectOpts := make([]prompt.SelectOption, len(skillLocations)+1)
	for i, loc := range skillLocations {
		selectOpts[i] = prompt.SelectOption{Label: loc.Label + " — " + loc.Path, Value: loc.Path}
	}
	selectOpts[len(skillLocations)] = prompt.SelectOption{Label: "Print to stdout", Value: selectValPrint}

	chosen, err := selectFunc("Install skill to:", selectOpts, opts.In(), opts.Err(), opts.NoInput)
	if err != nil {
		return err
	}

	if chosen == selectValPrint {
		_, err := opts.Out().Write(content)
		return err
	}

	return writeSkillFile(expandPath(chosen), content, opts)
}

func newInstallCmd() *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the skill to default agent locations",
		Long: `Install the embedded SKILL.md to ~/.agents/skills/gumroad/ (shared baseline).

If Claude Code is detected (~/.claude exists), creates a directory symlink at
~/.claude/skills/gumroad pointing to the shared location. Falls back to a
file copy if symlinks are unavailable.`,
		Example: `  # Install to default locations
  gumroad skill install

  # Install to a custom path
  gumroad skill install --path /custom/path/SKILL.md`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return runInstall(opts, path)
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Custom install path")
	return cmd
}

func runInstall(opts cmdutil.Options, customPath string) error {
	content, err := readSkill()
	if err != nil {
		return fmt.Errorf("could not read embedded skill: %w", err)
	}

	if customPath != "" {
		return writeSkillFile(customPath, content, opts)
	}

	home, err := userHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	// Write to the shared baseline
	sharedDir := filepath.Join(home, ".agents", skillDirName)
	sharedFile := filepath.Join(sharedDir, skillFileName)
	if err := writeSkillFile(sharedFile, content, opts); err != nil {
		return err
	}

	// Symlink Claude Code to the shared baseline (directory-level, relative path)
	claudeDir := filepath.Join(home, ".claude")
	if info, statErr := os.Stat(claudeDir); statErr == nil && info.IsDir() {
		if err := linkOrCopySkillDir(
			filepath.Join(home, ".claude", skillDirName),
			filepath.Join("..", "..", ".agents", skillDirName),
			sharedDir,
			opts,
		); err != nil {
			return err
		}
	}

	return nil
}

// writeSkillFile writes the skill content to a file path.
func writeSkillFile(path string, content []byte, opts cmdutil.Options) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G301: skill files are not secrets
		return fmt.Errorf("could not create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil { //nolint:gosec // G306: skill files are not secrets
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return cmdutil.PrintSuccess(opts, fmt.Sprintf("Installed skill to %s", path))
}

// linkOrCopySkillDir creates a directory symlink at linkPath pointing to relTarget.
// If symlinks fail (e.g. Windows), falls back to copying files from absTarget.
func linkOrCopySkillDir(linkPath, relTarget, absTarget string, opts cmdutil.Options) error {
	parent := filepath.Dir(linkPath)
	if err := os.MkdirAll(parent, 0755); err != nil { //nolint:gosec // G301: skill files are not secrets
		return fmt.Errorf("could not create directory %s: %w", parent, err)
	}

	// Remove existing symlink or file. If it's a populated directory,
	// leave it alone — the user may have placed custom files there.
	if info, err := os.Lstat(linkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			_ = os.Remove(linkPath)
		} else {
			// It's a real directory — skip symlink, fall through to copy fallback
			return copySkillDir(absTarget, linkPath, opts)
		}
	}

	if err := os.Symlink(relTarget, linkPath); err == nil {
		return cmdutil.PrintSuccess(opts, fmt.Sprintf("Linked %s → %s", linkPath, relTarget))
	}

	// Fallback: copy files from the shared directory
	return copySkillDir(absTarget, linkPath, opts)
}

// copySkillDir copies all files from src to dst as a fallback when symlinks fail.
func copySkillDir(src, dst string, opts cmdutil.Options) error {
	if err := os.MkdirAll(dst, 0755); err != nil { //nolint:gosec // G301: skill files are not secrets
		return fmt.Errorf("could not create directory %s: %w", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", src, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(src, e.Name()))
		if readErr != nil {
			return fmt.Errorf("could not read %s: %w", e.Name(), readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(dst, e.Name()), data, 0644); writeErr != nil { //nolint:gosec // G306: skill files are not secrets
			return fmt.Errorf("could not write %s: %w", e.Name(), writeErr)
		}
	}

	return cmdutil.PrintSuccess(opts, fmt.Sprintf("Copied skill to %s (symlink unavailable)", dst))
}
