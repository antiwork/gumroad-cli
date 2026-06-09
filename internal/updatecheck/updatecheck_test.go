package updatecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

func testOptions(stderr *bytes.Buffer) cmdutil.Options {
	opts := cmdutil.DefaultOptions()
	opts.Version = "v1.0.0"
	opts.Stderr = stderr
	return opts
}

func replaceConfigDir(t *testing.T, dir string) {
	t.Helper()
	previous := configDir
	configDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDir = previous })
}

func replaceNow(t *testing.T, t0 time.Time) {
	t.Helper()
	previous := now
	now = func() time.Time { return t0 }
	t.Cleanup(func() { now = previous })
}

func replaceLatestVersion(t *testing.T, fn func(context.Context) (string, error)) {
	t.Helper()
	previous := latestVersion
	latestVersion = fn
	t.Cleanup(func() { latestVersion = previous })
}

func replaceStartRefresh(t *testing.T, fn func()) {
	t.Helper()
	previous := startRefresh
	startRefresh = fn
	t.Cleanup(func() { startRefresh = previous })
}

func replaceExecutable(t *testing.T, path string, resolved string) {
	t.Helper()
	previousExecutable := executable
	previousEvalSymlinks := evalSymlinks
	executable = func() (string, error) { return path, nil }
	evalSymlinks = func(string) (string, error) { return resolved, nil }
	t.Cleanup(func() {
		executable = previousExecutable
		evalSymlinks = previousEvalSymlinks
	})
}

func replaceHomeDir(t *testing.T, path string) {
	t.Helper()
	previous := userHomeDir
	userHomeDir = func() (string, error) { return path, nil }
	t.Cleanup(func() { userHomeDir = previous })
}

func readCache(t *testing.T, dir string) cacheState {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, cacheFileName))
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var state cacheState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse cache: %v", err)
	}
	return state
}

func writeCache(t *testing.T, dir string, state cacheState) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), data, 0600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

func TestNotifyPrintsWarningAndCachesNotice(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion: "v1.1.0",
		CheckedAt:     t0,
	})
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	var stderr bytes.Buffer
	Notify(testOptions(&stderr), "gumroad products list")

	got := stderr.String()
	if !strings.Contains(got, "warning: gumroad v1.1.0 is available; you have v1.0.0.") {
		t.Fatalf("expected update warning, got %q", got)
	}
	if !strings.Contains(got, "Update:") {
		t.Fatalf("expected update command, got %q", got)
	}

	state := readCache(t, dir)
	if state.LatestVersion != "v1.1.0" || state.LastNoticeVersion != "v1.1.0" {
		t.Fatalf("unexpected cache state: %+v", state)
	}
	if !state.CheckedAt.Equal(t0) || !state.LastNoticeAt.Equal(t0) {
		t.Fatalf("unexpected cache timestamps: %+v", state)
	}
}

func TestNotifyDoesNotRepeatSameVersionImmediately(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion:     "v1.1.0",
		CheckedAt:         t0,
		LastNoticeVersion: "v1.1.0",
		LastNoticeAt:      t0,
	})
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	var stderr bytes.Buffer
	Notify(testOptions(&stderr), "gumroad products list")

	if stderr.Len() != 0 {
		t.Fatalf("expected no repeated warning, got %q", stderr.String())
	}
}

func TestNotifyRepeatsSameVersionAfterNoticeInterval(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0.Add(noticeInterval))
	writeCache(t, dir, cacheState{
		LatestVersion:     "v1.1.0",
		CheckedAt:         t0.Add(noticeInterval),
		LastNoticeVersion: "v1.1.0",
		LastNoticeAt:      t0,
	})
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	var stderr bytes.Buffer
	Notify(testOptions(&stderr), "gumroad products list")

	if !strings.Contains(stderr.String(), "v1.1.0 is available") {
		t.Fatalf("expected repeated warning after interval, got %q", stderr.String())
	}
}

func TestNotifyUsesFreshCachedLatestWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion: "v1.1.0",
		CheckedAt:     t0,
	})
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	var stderr bytes.Buffer
	Notify(testOptions(&stderr), "gumroad sales list")

	if !strings.Contains(stderr.String(), "v1.1.0 is available") {
		t.Fatalf("expected cached warning, got %q", stderr.String())
	}
}

func TestNotifyDisplaysDateBasedVersions(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion: "v0.20260609.0",
		CheckedAt:     t0,
	})
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	opts := testOptions(&bytes.Buffer{})
	opts.Version = "0.20260608.0"

	var stderr bytes.Buffer
	opts.Stderr = &stderr
	Notify(opts, "gumroad products list")

	if !strings.Contains(stderr.String(), "warning: gumroad 2026.06.09 is available; you have 2026.06.08.") {
		t.Fatalf("expected date-based warning, got %q", stderr.String())
	}
}

func TestNotifyTreatsDateBasedVersionAsNewerThanLegacySemver(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion: "v0.20260609.0",
		CheckedAt:     t0,
	})
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	opts := testOptions(&bytes.Buffer{})
	opts.Version = "0.21.0"

	var stderr bytes.Buffer
	opts.Stderr = &stderr
	Notify(opts, "gumroad products list")

	if !strings.Contains(stderr.String(), "warning: gumroad 2026.06.09 is available; you have 0.21.0.") {
		t.Fatalf("expected date release to supersede legacy semver, got %q", stderr.String())
	}
}

func TestNotifyStartsRefreshWithoutBlockingForStaleCache(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)

	called := false
	replaceStartRefresh(t, func() {
		called = true
	})

	var stderr bytes.Buffer
	Notify(testOptions(&stderr), "gumroad products list")

	if !called {
		t.Fatal("expected stale cache to start refresh")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no first-run warning from live refresh, got %q", stderr.String())
	}
	state := readCache(t, dir)
	if !state.CheckedAt.Equal(t0) {
		t.Fatalf("expected checked_at throttle marker before refresh, got %+v", state)
	}
}

func TestNotifySuppressesQuietCompletionAndDevBuilds(t *testing.T) {
	for _, tc := range []struct {
		name        string
		opts        cmdutil.Options
		commandPath string
	}{
		{name: "quiet", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "v1.0.0"
			opts.Quiet = true
			return opts
		}(), commandPath: "gumroad products list"},
		{name: "completion", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "v1.0.0"
			return opts
		}(), commandPath: "gumroad completion bash"},
		{name: "cobra complete", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "v1.0.0"
			return opts
		}(), commandPath: "gumroad __complete products list"},
		{name: "cobra complete no desc", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "v1.0.0"
			return opts
		}(), commandPath: "gumroad __completeNoDesc products list"},
		{name: "background refresh", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "v1.0.0"
			return opts
		}(), commandPath: "gumroad __update-check-refresh"},
		{name: "dev", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "dev"
			return opts
		}(), commandPath: "gumroad products list"},
		{name: "test", opts: func() cmdutil.Options {
			opts := cmdutil.DefaultOptions()
			opts.Version = "test"
			return opts
		}(), commandPath: "gumroad products list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replaceLatestVersion(t, func(context.Context) (string, error) {
				t.Fatal("suppressed update check should not fetch")
				return "", nil
			})

			var stderr bytes.Buffer
			tc.opts.Stderr = &stderr
			Notify(tc.opts, tc.commandPath)

			if stderr.Len() != 0 {
				t.Fatalf("expected no output, got %q", stderr.String())
			}
		})
	}
}

func TestNotifyRefreshFailureIsSilent(t *testing.T) {
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion: "v1.1.0",
		CheckedAt:     t0.Add(-checkInterval),
	})
	replaceLatestVersion(t, func(context.Context) (string, error) {
		return "", errors.New("offline")
	})

	if err := refreshCacheOnce(context.Background(), filepath.Join(dir, cacheFileName), cacheState{}, t0); err != nil {
		t.Fatalf("refresh cache: %v", err)
	}

	state := readCache(t, dir)
	if !state.CheckedAt.Equal(t0) {
		t.Fatalf("expected failed check to be cached, got %+v", state)
	}
	if state.LatestVersion != "v1.1.0" {
		t.Fatalf("expected failed check to preserve latest version, got %+v", state)
	}
}

func TestNotifyIgnoresOlderOrMalformedVersions(t *testing.T) {
	for _, latest := range []string{"v0.9.0", "not-semver"} {
		t.Run(latest, func(t *testing.T) {
			dir := t.TempDir()
			replaceConfigDir(t, dir)
			t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
			replaceNow(t, t0)
			writeCache(t, dir, cacheState{
				LatestVersion: latest,
				CheckedAt:     t0,
			})
			replaceStartRefresh(t, func() {
				t.Fatal("fresh cache should not refresh")
			})

			var stderr bytes.Buffer
			Notify(testOptions(&stderr), "gumroad products list")

			if stderr.Len() != 0 {
				t.Fatalf("expected no warning, got %q", stderr.String())
			}
		})
	}
}

func TestNotifySkipsWarningWhenNoticeCannotBePersisted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based write failure is Unix-specific")
	}
	dir := t.TempDir()
	replaceConfigDir(t, dir)
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	replaceNow(t, t0)
	writeCache(t, dir, cacheState{
		LatestVersion: "v1.1.0",
		CheckedAt:     t0,
	})
	path := filepath.Join(dir, cacheFileName)
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatalf("chmod cache file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	replaceStartRefresh(t, func() {
		t.Fatal("fresh cache should not refresh")
	})

	var stderr bytes.Buffer
	Notify(testOptions(&stderr), "gumroad products list")

	if stderr.Len() != 0 {
		t.Fatalf("expected no warning when notice marker cannot persist, got %q", stderr.String())
	}
}

func TestRefreshCacheOncePreservesNoticeFields(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	noticeAt := t0.Add(-time.Hour)
	writeCache(t, dir, cacheState{
		LatestVersion:     "v1.1.0",
		CheckedAt:         t0.Add(-checkInterval),
		LastNoticeVersion: "v1.1.0",
		LastNoticeAt:      noticeAt,
	})
	replaceLatestVersion(t, func(context.Context) (string, error) {
		return "v1.2.0", nil
	})

	if err := refreshCacheOnce(context.Background(), filepath.Join(dir, cacheFileName), cacheState{}, t0); err != nil {
		t.Fatalf("refresh cache: %v", err)
	}

	state := readCache(t, dir)
	if state.LatestVersion != "v1.2.0" || !state.CheckedAt.Equal(t0) {
		t.Fatalf("expected refreshed latest version, got %+v", state)
	}
	if state.LastNoticeVersion != "v1.1.0" || !state.LastNoticeAt.Equal(noticeAt) {
		t.Fatalf("expected notice fields to be preserved, got %+v", state)
	}
}

func TestUpdateCommandDetectsInstallMethod(t *testing.T) {
	tmp := t.TempDir()
	replaceHomeDir(t, filepath.Join(tmp, "home"))

	tests := []struct {
		name     string
		path     string
		resolved string
		env      map[string]string
		want     string
	}{
		{
			name:     "homebrew",
			path:     "/opt/homebrew/bin/gumroad",
			resolved: "/opt/homebrew/Cellar/gumroad/1.1.0/bin/gumroad",
			want:     "brew upgrade antiwork/cli/gumroad",
		},
		{
			name:     "go gobin",
			path:     filepath.Join(tmp, "gobin", "gumroad"),
			resolved: filepath.Join(tmp, "gobin", "gumroad"),
			env:      map[string]string{"GOBIN": filepath.Join(tmp, "gobin")},
			want:     "go install github.com/antiwork/gumroad-cli/cmd/gumroad@latest",
		},
		{
			name:     "go default",
			path:     filepath.Join(tmp, "home", "go", "bin", "gumroad"),
			resolved: filepath.Join(tmp, "home", "go", "bin", "gumroad"),
			want:     "go install github.com/antiwork/gumroad-cli/cmd/gumroad@latest",
		},
		{
			name:     "source checkout",
			path:     filepath.Join(tmp, "gumroad-cli", "gumroad"),
			resolved: filepath.Join(tmp, "gumroad-cli", "gumroad"),
			want:     "git pull && make install",
		},
		{
			name:     "unknown",
			path:     filepath.Join(tmp, ".local", "bin", "gumroad"),
			resolved: filepath.Join(tmp, ".local", "bin", "gumroad"),
			want:     "curl -fsSL https://gumroad.com/install-cli.sh | bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOBIN", "")
			t.Setenv("GOPATH", "")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			replaceExecutable(t, tt.path, tt.resolved)

			if got := updateCommand(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
