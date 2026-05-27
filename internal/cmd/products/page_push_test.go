package products

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPagePushPublishesHTMLAndSavesSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	htmlPath := writePageHTML(t, "<main>New</main>")

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/products/prod1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["custom_html"] != "<main>New</main>" {
			t.Fatalf("custom_html = %q", body["custom_html"])
		}
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"custom_html": "<main>New</main>",
				"landing_url": "https://creator.example/l/prod1",
			},
			"previous_custom_html": "<main>Old</main>",
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePushCmd(), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() {
		cmd.SetArgs([]string{"prod1", htmlPath})
		testutil.MustExecute(t, cmd)
	})

	if !strings.Contains(out, "Live at https://creator.example/l/prod1") {
		t.Fatalf("missing live URL: %q", out)
	}
	snapshots := findSnapshotFiles(t, home)
	if len(snapshots) != 1 {
		t.Fatalf("got %d snapshots, want 1", len(snapshots))
	}
	data, err := os.ReadFile(snapshots[0])
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(data) != "<main>Old</main>" {
		t.Fatalf("snapshot = %q", data)
	}
}

func TestPagePushDryRunUsesPreviewEndpoint(t *testing.T) {
	htmlPath := writePageHTML(t, "<main>Preview</main>")
	var sawPreview bool

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/products/prod1/preview_custom_html" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawPreview = true
		testutil.JSON(t, w, map[string]any{
			"custom_html": "<main>Preview</main>",
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePushCmd(), testutil.DryRun(true))
	cmd.SetArgs([]string{"prod1", htmlPath})
	testutil.MustExecute(t, cmd)
	if !sawPreview {
		t.Fatal("preview endpoint was not called")
	}
}

func TestPagePushReadsStdin(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["custom_html"] != "<main>stdin</main>" {
			t.Fatalf("custom_html = %q", body["custom_html"])
		}
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"custom_html": "<main>stdin</main>",
				"landing_url": "https://creator.example/l/prod1",
			},
			"previous_custom_html": nil,
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePushCmd(), testutil.Stdin(strings.NewReader("<main>stdin</main>")))
	cmd.SetArgs([]string{"prod1", "-"})
	testutil.MustExecute(t, cmd)
}

func TestPagePushRateLimitUsesPageHint(t *testing.T) {
	htmlPath := writePageHTML(t, "<main>New</main>")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		testutil.RawJSON(t, w, `{"success":false,"message":"Rate limited"}`)
	})

	cmd := testutil.Command(newPagePushCmd())
	cmd.SetArgs([]string{"prod1", htmlPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "Hit Gumroad's rate limit (30 PUTs/min per token).") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPagePushQuietSuppressesHumanOutput(t *testing.T) {
	htmlPath := writePageHTML(t, "<main>New</main>")
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"custom_html": "<main>New</main>",
				"landing_url": "https://creator.example/l/prod1",
			},
			"previous_custom_html": nil,
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPagePushCmd(), testutil.Quiet(true))
	cmd.SetArgs([]string{"prod1", htmlPath})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if out != "" {
		t.Fatalf("expected quiet push to suppress stdout, got %q", out)
	}
}

func writePageHTML(t *testing.T, html string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "landing.html")
	if err := os.WriteFile(path, []byte(html), 0600); err != nil {
		t.Fatalf("write html: %v", err)
	}
	return path
}

func findSnapshotFiles(t *testing.T, home string) []string {
	t.Helper()
	var paths []string
	root := filepath.Join(home, ".gumroad", "pages")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk snapshots: %v", err)
	}
	return paths
}
