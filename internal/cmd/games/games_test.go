package games

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func gameDir(t *testing.T) string {
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
	zipPath, err := zipDir(gameDir(t))
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

func TestDeploy_UploadsZipThenDeploysAndPromotes(t *testing.T) {
	s3 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"e1"`)
	}))
	defer s3.Close()
	prev := s3HTTPClientForTesting
	s3HTTPClientForTesting = s3.Client()
	defer func() { s3HTTPClientForTesting = prev }()

	var deployURL string
	var promoted atomic.Bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/files/presign":
			testutil.JSON(t, w, map[string]any{
				"upload_id": "up", "key": "attachments/u/k/original/game-build.zip",
				"file_url": "https://example.com/attachments/u/k/original/game-build.zip",
				"parts":    []map[string]any{{"part_number": 1, "presigned_url": s3.URL + "/p/1"}},
			})
		case "/files/complete":
			testutil.JSON(t, w, map[string]any{"file_url": "https://example.com/attachments/u/k/original/game-build.zip"})
		case "/products/prod1/games/builds/deploy":
			deployURL = r.PostForm.Get("url")
			testutil.JSON(t, w, map[string]any{"success": true, "game_build": map[string]any{"id": "b1", "number": 1, "status": "uploaded"}})
		case "/products/prod1/games/builds/b1/promote":
			promoted.Store(true)
			testutil.JSON(t, w, map[string]any{"success": true, "game_build": map[string]any{"id": "b1", "number": 1, "status": "live"}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	})

	cmd := testutil.Command(newDeployCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{gameDir(t), "--product", "prod1", "--promote"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if deployURL != "https://example.com/attachments/u/k/original/game-build.zip" {
		t.Errorf("deploy url = %q", deployURL)
	}
	if !promoted.Load() {
		t.Error("expected promote call")
	}
	if !strings.Contains(out, `"live"`) {
		t.Errorf("out = %q", out)
	}
}

func TestDeploy_RequiresProduct(t *testing.T) {
	cmd := testutil.Command(newDeployCmd())
	cmd.SetArgs([]string{gameDir(t)})
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
		case r.Method == "POST" && r.URL.Path == "/products/p/games/builds/b2/promote":
			testutil.JSON(t, w, map[string]any{"success": true})
		case r.Method == "GET" && r.URL.Path == "/products/p/games/builds":
			testutil.JSON(t, w, map[string]any{"success": true, "game_builds": []map[string]any{{"id": "b2", "number": 2, "status": "live", "byte_size": 10}}})
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
	if !strings.Contains(out, "b2") || !strings.Contains(out, `"live"`) {
		t.Errorf("out = %q", out)
	}
}
