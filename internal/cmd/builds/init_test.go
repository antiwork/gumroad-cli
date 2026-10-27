package builds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func readStarter(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestInit_CreatesStarterInNewDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-game")
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--product", "abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var got struct {
		Success bool     `json:"success"`
		Files   []string `json:"files"`
		Next    string   `json:"next"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("out = %q: %v", out, err)
	}
	if !got.Success || len(got.Files) != 2 {
		t.Fatalf("out = %q", out)
	}

	index := readStarter(t, filepath.Join(dir, starterIndexFile))
	if !strings.Contains(index, "<canvas") {
		t.Errorf("index.html is not a playable starter: %q", index)
	}
	script := readStarter(t, filepath.Join(dir, starterDeployFile))
	if !strings.Contains(script, "gumroad builds deploy . --product \"$product\" --promote") {
		t.Errorf("deploy.sh does not deploy this folder: %q", script)
	}
	if !strings.Contains(script, "product='abc123'") {
		t.Errorf("deploy.sh does not carry --product: %q", script)
	}
	if want := "gumroad builds deploy '" + dir + "' --product 'abc123' --promote"; got.Next != want {
		t.Errorf("next = %q, want %q", got.Next, want)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, starterDeployFile))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("deploy.sh is not executable: %v", info.Mode().Perm())
		}
	}
}

func TestInit_WithoutProductReadsEnvironment(t *testing.T) {
	dir := t.TempDir()
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	script := readStarter(t, filepath.Join(dir, starterDeployFile))
	if !strings.Contains(script, "${GUMROAD_PRODUCT:?") {
		t.Errorf("deploy.sh should require GUMROAD_PRODUCT: %q", script)
	}
}

func TestInit_RefusesToOverwriteEitherFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, starterIndexFile)
	if err := os.WriteFile(existing, []byte("<html>mine</html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := testutil.Command(newInitCmd())
	cmd.SetArgs([]string{dir})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
	if got := readStarter(t, existing); got != "<html>mine</html>" {
		t.Errorf("index.html was overwritten: %q", got)
	}
	// The check runs before any write, so the refusal creates nothing.
	if _, statErr := os.Stat(filepath.Join(dir, starterDeployFile)); statErr == nil {
		t.Error("deploy.sh was created despite the refusal")
	}
}

func TestInit_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, starterIndexFile), []byte("<html>mine</html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--force"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := readStarter(t, filepath.Join(dir, starterIndexFile)); !strings.Contains(got, "<canvas") {
		t.Errorf("index.html was not replaced: %q", got)
	}
}

func TestInit_DryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-game")
	cmd := testutil.Command(newInitCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--product", "abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var got struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("out = %q: %v", out, err)
	}
	want := "create " + filepath.Join(dir, starterIndexFile) + " and " + filepath.Join(dir, starterDeployFile)
	if !got.DryRun || got.Action != want {
		t.Errorf("dry run = %+v, want action %q", got, want)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("dry run created the directory")
	}
}

func TestInit_StarterDeploysAsIs(t *testing.T) {
	dir := t.TempDir()
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--product", "abc123"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	// The starter is only useful if the deploy path accepts it unchanged.
	zipPath, err := zipDir(dir)
	if err != nil {
		t.Fatalf("zipDir(starter) = %v", err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	if got, want := zipEntries(t, zipPath), []string{starterDeployFile, starterIndexFile}; !slices.Equal(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

func TestInit_QuotesProductInDeployScript(t *testing.T) {
	dir := t.TempDir()
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--product", "it's"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	script := readStarter(t, filepath.Join(dir, starterDeployFile))
	if !strings.Contains(script, `product='it'\''s'`) {
		t.Errorf("deploy.sh does not quote the product: %q", script)
	}
}

func TestInit_RejectsDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	outside := filepath.Join(t.TempDir(), "outside.html")
	if err := os.WriteFile(outside, []byte("<html>untouched</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, starterIndexFile)); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{dir}, {dir, "--force"}} {
		cmd := testutil.Command(newInitCmd())
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("args %v: err = %v", args, err)
		}
	}
	if got := readStarter(t, outside); got != "<html>untouched</html>" {
		t.Errorf("init wrote through the link: %q", got)
	}
}

func TestInit_StarterScriptDeclaresEachConstantOnce(t *testing.T) {
	script := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindStringSubmatch(starterIndexHTML)
	if script == nil {
		t.Fatal("starter has no inline script")
	}
	// A repeated top-level const is a SyntaxError, so nothing in the page runs.
	seen := map[string]bool{}
	for _, match := range regexp.MustCompile(`\bconst ([A-Za-z_$][\w$]*)`).FindAllStringSubmatch(script[1], -1) {
		if seen[match[1]] {
			t.Errorf("starter declares const %s twice", match[1])
		}
		seen[match[1]] = true
	}
	if len(seen) == 0 {
		t.Error("starter script declares no constants")
	}
}
