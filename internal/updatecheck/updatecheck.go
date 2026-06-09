package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/version"
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
	startRefresh  = startRefreshProcess
)

const RefreshCommandName = "__update-check-refresh"

type cacheState struct {
	LatestVersion     string    `json:"latest_version,omitempty"`
	CheckedAt         time.Time `json:"checked_at,omitempty"`
	LastNoticeVersion string    `json:"last_notice_version,omitempty"`
	LastNoticeAt      time.Time `json:"last_notice_at,omitempty"`
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
}

// Notify prints a low-noise update notice to stderr when a newer release exists.
// It never returns an error; update checks should not affect the requested command.
func Notify(opts cmdutil.Options, commandPath string) {
	if !eligible(opts, commandPath) {
		return
	}

	current, ok := version.Parse(opts.Version)
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
		if err := saveCache(path, state); err != nil {
			return
		}
		defer startRefresh()
	}

	latest, ok := version.Parse(state.LatestVersion)
	if !ok || version.Compare(latest, current) <= 0 || !shouldShowNotice(state, t) {
		return
	}

	state.LastNoticeVersion = state.LatestVersion
	state.LastNoticeAt = t
	if err := saveCache(path, state); err != nil {
		return
	}
	fmt.Fprintf(opts.Err(), "warning: gumroad %s is available; you have %s. Update: %s\n", version.Display(state.LatestVersion), version.Display(opts.Version), updateCommand())
}

// Refresh updates the cache for the hidden background refresh command. It is
// intentionally silent; update checks must never affect the foreground command.
func Refresh(ctx context.Context) {
	path, state, ok := loadCache()
	if !ok {
		return
	}
	_ = refreshCacheOnce(ctx, path, state, now())
}

func IsRefreshCommand(commandPath string) bool {
	fields := strings.Fields(commandPath)
	for _, field := range fields {
		if field == RefreshCommandName {
			return true
		}
	}
	return false
}

func eligible(opts cmdutil.Options, commandPath string) bool {
	if opts.Quiet {
		return false
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" || version == "dev" || version == "test" {
		return false
	}
	return !isCompletionCommand(commandPath) && !IsRefreshCommand(commandPath)
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
	state, ok, err := readCacheFile(path)
	if err != nil {
		return path, cacheState{}, os.IsNotExist(err)
	}
	if !ok {
		return path, cacheState{}, true
	}
	return path, state, true
}

func readCacheFile(path string) (cacheState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheState{}, false, err
	}
	var state cacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return cacheState{}, false, nil
	}
	return state, true, nil
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

func startRefreshProcess() {
	path, err := executable()
	if err != nil {
		return
	}
	cmd := exec.Command(path, RefreshCommandName) //nolint:gosec // path is the current executable, not user input
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release()
}

func refreshCacheOnce(parent context.Context, path string, state cacheState, checkedAt time.Time) error {
	state.CheckedAt = checkedAt
	latest, hasLatest := "", false
	if fetched, err := checkLatest(parent); err == nil && fetched != "" {
		latest = fetched
		hasLatest = true
		state.LatestVersion = fetched
	}
	if current, ok, err := readCacheFile(path); err == nil && ok {
		current.CheckedAt = state.CheckedAt
		if hasLatest {
			current.LatestVersion = latest
		}
		state = current
	}
	return saveCache(path, state)
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
