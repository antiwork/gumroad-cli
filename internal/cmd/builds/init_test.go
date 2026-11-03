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

func TestInit_FrameworkStartersWriteADeployableFolder(t *testing.T) {
	for _, name := range frameworkOrder {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
			cmd.SetArgs([]string{dir, "--framework", name, "--product", "abc123"})
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			var got struct {
				Files []string `json:"files"`
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("out = %q: %v", out, err)
			}
			if len(got.Files) == 0 {
				t.Fatalf("no files reported: %q", out)
			}
			script := readStarter(t, filepath.Join(dir, starterDeployFile))
			if !strings.Contains(script, "gumroad builds deploy . --product \"$product\" --promote") {
				t.Errorf("deploy.sh does not deploy this folder: %q", script)
			}
			if !strings.Contains(script, "product='abc123'") {
				t.Errorf("deploy.sh does not carry --product: %q", script)
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
			// A page that loads a script from a public CDN works locally and
			// then fails in the player, whose CSP allows scripts only from the
			// app host and the asset host.
			for _, file := range got.Files {
				if !strings.HasSuffix(file, ".html") {
					continue
				}
				if page := readStarter(t, file); strings.Contains(page, `src="http`) {
					t.Errorf("%s loads a script from a remote host: %q", file, page)
				}
			}
		})
	}
}

func TestInit_ThreeVendorsTheWholeModule(t *testing.T) {
	dir := t.TempDir()
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--framework", "three"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var got struct {
		Setup string `json:"setup"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("out = %q: %v", out, err)
	}
	if want := "./" + starterSetupFile; got.Setup != want {
		t.Errorf("setup = %q, want %q", got.Setup, want)
	}
	index := readStarter(t, filepath.Join(dir, starterIndexFile))
	if !strings.Contains(index, `"three": "./vendor/three.module.js"`) {
		t.Errorf("index.html does not map three to the vendored file: %q", index)
	}
	// three.module.js imports three.core.js, so vendoring only the entry point
	// leaves the page with no renderer.
	setup := readStarter(t, filepath.Join(dir, starterSetupFile))
	for _, want := range []string{"vendor/three.module.js", "vendor/three.core.js"} {
		if !strings.Contains(setup, want) {
			t.Errorf("setup.sh does not fetch %s: %q", want, setup)
		}
	}
	if main := readStarter(t, filepath.Join(dir, starterMainFile)); !strings.Contains(main, `from "three"`) {
		t.Errorf("main.js does not import three: %q", main)
	}
}

func TestInit_PhaserUsesNamedImports(t *testing.T) {
	dir := t.TempDir()
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--framework", "phaser"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	index := readStarter(t, filepath.Join(dir, starterIndexFile))
	if !strings.Contains(index, `"phaser": "./vendor/phaser.esm.min.js"`) {
		t.Errorf("index.html does not map phaser to the vendored file: %q", index)
	}
	main := readStarter(t, filepath.Join(dir, starterMainFile))
	// Phaser 4 exports names, not a default, so a default import is undefined.
	if strings.Contains(main, "import Phaser from") {
		t.Errorf("main.js default-imports phaser, which has no default export: %q", main)
	}
	if !strings.Contains(main, `import { AUTO, Game, Scene, Scale } from "phaser"`) {
		t.Errorf("main.js does not named-import phaser: %q", main)
	}
}

func TestInit_GodotAndUnityLeaveIndexHTMLToTheExporter(t *testing.T) {
	for _, name := range []string{"godot", "unity"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
			cmd.SetArgs([]string{dir, "--framework", name})
			testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			// The exporter writes index.html. A placeholder would be left next
			// to the export, or the export would overwrite it.
			if _, err := os.Stat(filepath.Join(dir, starterIndexFile)); err == nil {
				t.Error("init wrote an index.html the exporter owns")
			}
			readme := readStarter(t, filepath.Join(dir, starterReadmeFile))
			if !strings.Contains(strings.ToLower(readme), "thread") {
				t.Errorf("README does not mention the single-threaded export: %q", readme)
			}
		})
	}
}

func TestInit_RejectsUnknownFramework(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-game")
	cmd := testutil.Command(newInitCmd())
	cmd.SetArgs([]string{dir, "--framework", "unreal"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown framework") {
		t.Fatalf("err = %v", err)
	}
	for _, name := range frameworkOrder {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not list %s: %v", name, err)
		}
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Error("an unknown framework created the directory")
	}
}

func TestInit_FrameworkStarterZipsWithIndexHTMLAtTheTop(t *testing.T) {
	dir := t.TempDir()
	cmd := testutil.Command(newInitCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{dir, "--framework", "three"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	zipPath, err := zipDir(dir)
	if err != nil {
		t.Fatalf("zipDir(starter) = %v", err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	if entries := zipEntries(t, zipPath); !slices.Contains(entries, starterIndexFile) {
		t.Errorf("zip entries = %v, want %s at the top level", entries, starterIndexFile)
	}
}
