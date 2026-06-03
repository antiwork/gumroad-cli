package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
)

const (
	latestReleaseURL = "https://api.github.com/repos/antiwork/gumroad-cli/releases/latest"
	cacheFileName    = "update-check.json"
	checkInterval    = 24 * time.Hour
	noticeInterval   = 24 * time.Hour
	requestTimeout   = 750 * time.Millisecond
)

var (
	now           = time.Now
	configDir     = config.Dir
	executable    = os.Executable
	evalSymlinks  = filepath.EvalSymlinks
	userHomeDir   = os.UserHomeDir
	httpClient    = http.DefaultClient
	latestVersion = fetchLatestVersion
)

type cacheState struct {
	LatestVersion     string    `json:"latest_version,omitempty"`
	CheckedAt         time.Time `json:"checked_at,omitempty"`
	LastNoticeVersion string    `json:"last_notice_version,omitempty"`
	LastNoticeAt      time.Time `json:"last_notice_at,omitempty"`
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
}

type semver struct {
	Major int
	Minor int
	Patch int
}

// Notify prints a low-noise update notice to stderr when a newer release exists.
// It never returns an error; update checks should not affect the requested command.
func Notify(opts cmdutil.Options, commandPath string) {
	if !eligible(opts, commandPath) {
		return
	}

	current, ok := parseVersion(opts.Version)
	if !ok {
		return
	}

	path, state, ok := loadCache()
	if !ok {
		return
	}

	t := now()
	if shouldRefresh(state, t) {
		state.CheckedAt = t
		if latest, err := checkLatest(opts.Context); err == nil && latest != "" {
			state.LatestVersion = latest
		}
		_ = saveCache(path, state)
	}

	latest, ok := parseVersion(state.LatestVersion)
	if !ok || compareVersions(latest, current) <= 0 || !shouldShowNotice(state, t) {
		return
	}

	fmt.Fprintf(opts.Err(), "warning: gumroad %s is available; you have %s. Update: %s\n", state.LatestVersion, opts.Version, updateCommand())
	state.LastNoticeVersion = state.LatestVersion
	state.LastNoticeAt = t
	_ = saveCache(path, state)
}

func eligible(opts cmdutil.Options, commandPath string) bool {
	if opts.Quiet {
		return false
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" || version == "dev" || version == "test" {
		return false
	}
	return !isCompletionCommand(commandPath)
}

func isCompletionCommand(commandPath string) bool {
	fields := strings.Fields(commandPath)
	for _, field := range fields {
		if field == "completion" || field == "__complete" || field == "__completeNoDesc" {
			return true
		}
	}
	return false
}

func loadCache() (string, cacheState, bool) {
	dir, err := configDir()
	if err != nil {
		return "", cacheState{}, false
	}
	path := filepath.Join(dir, cacheFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return path, cacheState{}, os.IsNotExist(err)
	}
	var state cacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return path, cacheState{}, true
	}
	return path, state, true
}

func saveCache(path string, state cacheState) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func shouldRefresh(state cacheState, t time.Time) bool {
	return state.CheckedAt.IsZero() || t.Sub(state.CheckedAt) >= checkInterval
}

func shouldShowNotice(state cacheState, t time.Time) bool {
	return state.LastNoticeVersion != state.LatestVersion ||
		state.LastNoticeAt.IsZero() ||
		t.Sub(state.LastNoticeAt) >= noticeInterval
}

func checkLatest(parent context.Context) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	return latestVersion(ctx)
}

func fetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gumroad-cli")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release returned %s", resp.Status)
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return strings.TrimSpace(release.TagName), nil
}

func parseVersion(version string) (semver, bool) {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if i := strings.IndexAny(version, "-+"); i >= 0 {
		version = version[:i]
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, false
	}
	return semver{Major: major, Minor: minor, Patch: patch}, true
}

func compareVersions(a, b semver) int {
	switch {
	case a.Major != b.Major:
		return a.Major - b.Major
	case a.Minor != b.Minor:
		return a.Minor - b.Minor
	default:
		return a.Patch - b.Patch
	}
}

func updateCommand() string {
	switch detectInstallMethod() {
	case "homebrew":
		return "brew upgrade antiwork/cli/gumroad"
	case "go":
		return "go install github.com/antiwork/gumroad-cli/cmd/gumroad@latest"
	case "source":
		return "git pull && make install"
	default:
		return "curl -fsSL https://gumroad.com/install-cli.sh | bash"
	}
}

func detectInstallMethod() string {
	path, err := executable()
	if err != nil {
		return ""
	}
	paths := []string{filepath.Clean(path)}
	if resolved, err := evalSymlinks(path); err == nil && resolved != "" {
		paths = append(paths, filepath.Clean(resolved))
	}
	for _, p := range paths {
		normalized := filepath.ToSlash(p)
		if strings.Contains(normalized, "/Cellar/gumroad/") || strings.Contains(normalized, "/Cellar/gumroad-cli/") {
			return "homebrew"
		}
		if isGoInstallPath(p) {
			return "go"
		}
		if isSourceInstallPath(p) {
			return "source"
		}
	}
	return ""
}

func isGoInstallPath(path string) bool {
	if gobin := strings.TrimSpace(os.Getenv("GOBIN")); gobin != "" && samePath(filepath.Dir(path), gobin) {
		return true
	}
	if gopath := strings.TrimSpace(os.Getenv("GOPATH")); gopath != "" && samePath(filepath.Dir(path), filepath.Join(gopath, "bin")) {
		return true
	}
	home, err := userHomeDir()
	if err != nil {
		return false
	}
	return samePath(filepath.Dir(path), filepath.Join(home, "go", "bin"))
}

func isSourceInstallPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/gumroad-cli/")
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
