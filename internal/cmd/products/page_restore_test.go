package products

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageRestorePutsSnapshotAndSnapshotsCurrentHTML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	snapshotHTML := "<main>Snapshot</main>"
	if _, err := pageutil.SaveSnapshot("products", "prod1", &snapshotHTML); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/products/prod1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["custom_html"] != snapshotHTML {
			t.Fatalf("custom_html = %q", body["custom_html"])
		}
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"custom_html": snapshotHTML,
				"landing_url": "https://creator.example/l/prod1",
			},
			"previous_custom_html": "<main>Current</main>",
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPageRestoreCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1"})
	testutil.MustExecute(t, cmd)

	snapshots := findSnapshotFiles(t, home)
	var foundCurrent bool
	for _, path := range snapshots {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		if strings.Contains(string(data), "Current") {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatalf("restore did not snapshot current HTML; snapshots=%v", snapshots)
	}
}
