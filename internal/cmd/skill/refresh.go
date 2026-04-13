package skill

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/skills"
)

const sentinelFile = ".last-skill-version"

var configDir = config.Dir

// AutoRefresh silently refreshes installed skill files when the CLI version changes.
// It only overwrites files that already exist — it never creates new installs.
func AutoRefresh(version string) {
	if version == "" || version == "dev" {
		return
	}

	dir, err := configDir()
	if err != nil {
		return
	}

	sentinelPath := filepath.Join(dir, sentinelFile)
	stored, _ := os.ReadFile(sentinelPath)
	if strings.TrimSpace(string(stored)) == version {
		return
	}

	content, err := skills.SkillMarkdown()
	if err != nil {
		return
	}

	if !refreshExistingInstalls(content) {
		return
	}

	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(sentinelPath, []byte(version), 0600)
}

// refreshExistingInstalls uses os.Stat (follows symlinks) so symlinked targets
// get refreshed. Skips project-relative paths — no reliable project root at startup.
func refreshExistingInstalls(content []byte) bool {
	ok := true
	for _, loc := range skillLocations {
		// Skip project-relative paths
		if !strings.HasPrefix(loc.Path, "~/") && !filepath.IsAbs(loc.Path) {
			continue
		}

		p := expandPath(loc.Path)
		if _, err := os.Stat(p); err != nil {
			continue
		}

		if err := os.WriteFile(p, content, 0644); err != nil { //nolint:gosec // G306: skill files are not secrets
			ok = false
		}
	}
	return ok
}
