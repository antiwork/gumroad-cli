package builds

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func buildDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "a.js"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestZipDir_RootIndexAndSkipsDotfiles(t *testing.T) {
	zipPath, err := zipDir(buildDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "index.html") || !strings.Contains(got, "assets/a.js") || strings.Contains(got, ".env") {
		t.Errorf("zip entries = %s", got)
	}
}

func TestZipDir_RequiresIndexHTML(t *testing.T) {
	if _, err := zipDir(t.TempDir()); err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Errorf("err = %v", err)
	}
}

func zipEntries(t *testing.T, zipPath string) []string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	return names
}

func requireSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
}

func TestZipDir_FollowsSymlinkedRootDirectory(t *testing.T) {
	requireSymlinks(t)
	link := filepath.Join(t.TempDir(), "dist")
	if err := os.Symlink(buildDir(t), link); err != nil {
		t.Fatal(err)
	}
	zipPath, err := zipDir(link)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	if got, want := zipEntries(t, zipPath), []string{"assets/a.js", "index.html"}; !slices.Equal(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

func TestZipDir_RejectsSymlinkedIndexHTML(t *testing.T) {
	requireSymlinks(t)
	target := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(target, []byte("<html></html>"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	if _, err := zipDir(dir); err == nil || !strings.Contains(err.Error(), "index.html must be a regular file") {
		t.Errorf("err = %v", err)
	}
}

func TestZipDir_RejectsIndexHTMLDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "index.html", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := zipDir(dir); err == nil || !strings.Contains(err.Error(), "index.html must be a regular file") {
		t.Errorf("err = %v", err)
	}
}

func TestZipDir_RejectsSymlinkedAssetInsteadOfDroppingIt(t *testing.T) {
	requireSymlinks(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := buildDir(t)
	if err := os.Symlink(secret, filepath.Join(dir, "assets", "b.js")); err != nil {
		t.Fatal(err)
	}
	zipPath, err := zipDir(dir)
	if err == nil {
		_ = os.Remove(zipPath)
		t.Fatal("expected an error for a symlink inside the build")
	}
	if !strings.Contains(err.Error(), "assets/b.js is not a regular file or folder") {
		t.Errorf("err = %v", err)
	}
}

func TestZipDir_SkipsHiddenDirectoriesWithoutReadingThem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permissions")
	}
	dir := buildDir(t)
	hidden := filepath.Join(dir, "assets", ".cache")
	if err := os.MkdirAll(hidden, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "c.js"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hidden, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(hidden, 0700) })
	if _, err := os.ReadDir(hidden); err == nil {
		t.Skip("running with permissions that can read a mode 0 directory")
	}

	zipPath, err := zipDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	if got, want := zipEntries(t, zipPath), []string{"assets/a.js", "index.html"}; !slices.Equal(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

type deployServer struct {
	deployURL string
	promoted  atomic.Bool
}

func setupDeployServer(t *testing.T, promote http.HandlerFunc) *deployServer {
	t.Helper()
	s3 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"e1"`)
	}))
	t.Cleanup(s3.Close)
	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = s3.Client()
	t.Cleanup(func() { s3HTTPClientForTesting = prev })

	srv := &deployServer{}
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/files/presign":
			testutil.JSON(t, w, map[string]any{
				"upload_id": "up", "key": "attachments/u/k/original/build.zip",
				"file_url": "https://example.com/attachments/u/k/original/build.zip",
				"parts":    []map[string]any{{"part_number": 1, "presigned_url": s3.URL + "/p/1"}},
			})
		case "/files/complete":
			testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/attachments/u/k/original/build.zip"})
		case "/products/prod1/builds/deploy":
			srv.deployURL = r.PostForm.Get("url")
			testutil.JSON(t, w, map[string]any{"success": true, "build": map[string]any{"id": "b1", "number": 1, "status": "uploaded"}})
		case "/products/prod1/builds/b1/promote":
			srv.promoted.Store(true)
			promote(w, r)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	})
	return srv
}

func decodeDeployOutput(t *testing.T, out string) build {
	t.Helper()
	var got struct {
		Result buildResponse `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("out = %q: %v", out, err)
	}
	return got.Result.Build
}

func TestDeploy_UploadsZipThenDeploysAndPromotes(t *testing.T) {
	srv := setupDeployServer(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "build": map[string]any{"id": "b1", "number": 1, "status": "live"}})
	})

	cmd := testutil.Command(newDeployCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{buildDir(t), "--product", "prod1", "--promote"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.deployURL != "https://example.com/attachments/u/k/original/build.zip" {
		t.Errorf("deploy url = %q", srv.deployURL)
	}
	if !srv.promoted.Load() {
		t.Error("expected promote call")
	}
	if got := decodeDeployOutput(t, out); got.ID != "b1" || got.Status != "live" {
		t.Errorf(".result.build = %+v, out = %q", got, out)
	}
}

func TestDeploy_WithoutPromoteLeavesBuildUploaded(t *testing.T) {
	srv := setupDeployServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("promote called without --promote")
	})

	cmd := testutil.Command(newDeployCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{buildDir(t), "--product", "prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if srv.promoted.Load() {
		t.Error("expected no promote call")
	}
	if got := decodeDeployOutput(t, out); got.ID != "b1" || got.Number != 1 || got.Status != "uploaded" {
		t.Errorf(".result.build = %+v, out = %q", got, out)
	}
}

func TestDeploy_PromoteFailureNamesTheDeployedBuild(t *testing.T) {
	setupDeployServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"success":false,"message":"Build is still processing"}`))
	})

	cmd := testutil.Command(newDeployCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{buildDir(t), "--product", "prod1", "--promote"})
	var err error
	testutil.CaptureStdout(func() { err = cmd.Execute() })
	if err == nil {
		t.Fatal("expected promote failure")
	}
	if want := "build 1 was deployed (id b1) but promoting it failed; run `gumroad builds promote b1 --product prod1`"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to contain %q", err, want)
	}
}

func TestDeploy_DryRunDescribesTheDeployWithoutRequests(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	})
	dir := buildDir(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{dir, "--product", "prod1"}, "deploy " + dir + " to product prod1"},
		{[]string{dir, "--product", "prod1", "--promote"}, "deploy " + dir + " to product prod1 and promote it"},
	} {
		cmd := testutil.Command(newDeployCmd(), testutil.DryRun(true), testutil.JSONOutput())
		cmd.SetArgs(tc.args)
		out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
		var got struct {
			DryRun bool   `json:"dry_run"`
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("out = %q: %v", out, err)
		}
		if !got.DryRun || got.Action != tc.want {
			t.Errorf("dry run = %+v, want action %q", got, tc.want)
		}
	}
}

func TestDeploy_RejectsMissingPath(t *testing.T) {
	cmd := testutil.Command(newDeployCmd())
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "missing"), "--product", "p"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "could not stat") {
		t.Errorf("err = %v", err)
	}
}

func TestDeploy_RejectsDirectoryWithoutIndexHTML(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	})
	cmd := testutil.Command(newDeployCmd())
	cmd.SetArgs([]string{t.TempDir(), "--product", "p"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "has no index.html") {
		t.Errorf("err = %v", err)
	}
}

func TestZipDir_RejectsMissingDirectory(t *testing.T) {
	if _, err := zipDir(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "could not resolve") {
		t.Errorf("err = %v", err)
	}
}

func TestDeploy_RequiresProduct(t *testing.T) {
	cmd := testutil.Command(newDeployCmd())
	cmd.SetArgs([]string{buildDir(t)})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error without --product")
	}
}

func TestDeploy_RejectsNonZipFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "game.txt")
	_ = os.WriteFile(f, []byte("x"), 0600)
	cmd := testutil.Command(newDeployCmd())
	cmd.SetArgs([]string{f, "--product", "p"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for non-zip")
	}
}

func TestPromoteAndList(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/products/p/builds/b2/promote":
			testutil.JSON(t, w, map[string]any{"success": true})
		case r.Method == "GET" && r.URL.Path == "/products/p/builds":
			testutil.JSON(t, w, map[string]any{"success": true, "builds": []map[string]any{{"id": "b2", "number": 2, "status": "live", "byte_size": 10}}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	p := testutil.Command(newPromoteCmd())
	p.SetArgs([]string{"b2", "--product", "p"})
	testutil.MustExecute(t, p)

	l := testutil.Command(newListCmd(), testutil.PlainOutput())
	l.SetArgs([]string{"--product", "p"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, l) })
	if want := "b2\t2\tlive\t10 B\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestList_EmptyAndTableOutput(t *testing.T) {
	builds := []map[string]any{}
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"success": true, "builds": builds})
	})

	var out strings.Builder
	l := testutil.Command(newListCmd(), testutil.Quiet(false), testutil.Stdout(&out))
	l.SetArgs([]string{"--product", "p"})
	testutil.MustExecute(t, l)
	if want := "No builds found.\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}

	builds = []map[string]any{{"id": "b2", "number": 2, "status": "live", "byte_size": 2048}}
	out.Reset()
	l = testutil.Command(newListCmd(), testutil.NoColor(true), testutil.Stdout(&out))
	l.SetArgs([]string{"--product", "p"})
	testutil.MustExecute(t, l)
	for _, want := range []string{"ID", "BUILD", "STATUS", "SIZE", "b2", "live", "2.0 KB"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("out = %q, want it to contain %q", out.String(), want)
		}
	}
}

func TestPromote_DryRunSkipsTheRequest(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	})
	p := testutil.Command(newPromoteCmd(), testutil.DryRun(true), testutil.JSONOutput())
	p.SetArgs([]string{"b2", "--product", "p"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, p) })
	if !strings.Contains(out, "/products/p/builds/b2/promote") {
		t.Errorf("out = %q", out)
	}
}

func TestListAndPromote_RequireProduct(t *testing.T) {
	for name, cmd := range map[string]struct {
		c    func() *cobra.Command
		args []string
	}{
		"list":    {newListCmd, nil},
		"promote": {newPromoteCmd, []string{"b2"}},
	} {
		c := testutil.Command(cmd.c())
		c.SetArgs(cmd.args)
		if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "--product is required") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNewBuildsCmd_RegistersSubcommands(t *testing.T) {
	var names []string
	for _, c := range NewBuildsCmd().Commands() {
		names = append(names, c.Name())
	}
	if want := []string{"deploy", "list", "promote"}; !slices.Equal(names, want) {
		t.Errorf("subcommands = %v, want %v", names, want)
	}
}
