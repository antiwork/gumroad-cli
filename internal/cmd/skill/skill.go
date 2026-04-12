package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/antiwork/gumroad-cli/skills"
	"github.com/spf13/cobra"
)

const (
	skillRelPath   = "skills/gumroad/SKILL.md"
	selectValPrint = "" // sentinel: user chose "Print to stdout"
)

type installTarget struct {
	Label string
	Path  string
}

var userHomeDir = os.UserHomeDir
var readSkill = skills.SkillMarkdown

var defaultTargets = func() []installTarget {
	targets := []installTarget{
		{"Claude Code (Project)", filepath.Join(".claude", skillRelPath)},
	}

	home, _ := userHomeDir()
	if home == "" {
		return targets
	}

	return append([]installTarget{
		{"Agents (Shared)", filepath.Join(home, ".agents", skillRelPath)},
		{"Claude Code (Global)", filepath.Join(home, ".claude", skillRelPath)},
	}, append(targets,
		installTarget{"Codex (Global)", filepath.Join(home, ".codex", skillRelPath)},
		installTarget{"OpenCode (Global)", filepath.Join(home, ".opencode", skillRelPath)},
	)...)
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

	if !output.IsTTY() || opts.NoInput {
		_, err := opts.Out().Write(content)
		return err
	}

	targets := defaultTargets()
	selectOpts := make([]prompt.SelectOption, len(targets)+1)
	for i, t := range targets {
		selectOpts[i] = prompt.SelectOption{Label: t.Label + " — " + t.Path, Value: t.Path}
	}
	selectOpts[len(targets)] = prompt.SelectOption{Label: "Print to stdout", Value: selectValPrint}

	chosen, err := selectFunc("Install skill to:", selectOpts, opts.In(), opts.Err(), opts.NoInput)
	if err != nil {
		return err
	}

	if chosen == selectValPrint {
		_, err := opts.Out().Write(content)
		return err
	}

	return writeSkillFile(chosen, content, opts)
}

func newInstallCmd() *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the skill to default agent locations",
		Long: `Install the embedded SKILL.md to ~/.agents/skills/gumroad/SKILL.md (shared baseline).

If Claude Code is detected (~/.claude exists), also creates a symlink at
~/.claude/skills/gumroad/SKILL.md pointing to the shared location.`,
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

	sharedPath := filepath.Join(home, ".agents", skillRelPath)
	if err := writeSkillFile(sharedPath, content, opts); err != nil {
		return err
	}

	claudeDir := filepath.Join(home, ".claude")
	if info, statErr := os.Stat(claudeDir); statErr == nil && info.IsDir() {
		claudeSkillPath := filepath.Join(home, ".claude", skillRelPath)
		if err := symlinkSkillFile(claudeSkillPath, sharedPath, opts); err != nil {
			return err
		}
	}

	return nil
}

func writeSkillFile(path string, content []byte, opts cmdutil.Options) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return cmdutil.PrintSuccess(opts, fmt.Sprintf("Installed skill to %s", path))
}

func symlinkSkillFile(linkPath, targetPath string, opts cmdutil.Options) error {
	dir := filepath.Dir(linkPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", dir, err)
	}

	if err := os.Remove(linkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove existing %s: %w", linkPath, err)
	}

	if err := os.Symlink(targetPath, linkPath); err != nil {
		return fmt.Errorf("could not symlink %s → %s: %w", linkPath, targetPath, err)
	}
	return cmdutil.PrintSuccess(opts, fmt.Sprintf("Linked %s → %s", linkPath, targetPath))
}
