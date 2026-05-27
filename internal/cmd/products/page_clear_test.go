package products

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageClearSendsNullAndSnapshotsPreviousHTML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/products/prod1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := body["custom_html"]; !ok || body["custom_html"] != nil {
			t.Fatalf("custom_html was not null: %#v", body)
		}
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{
				"custom_html": nil,
				"landing_url": "https://creator.example/l/prod1",
			},
			"previous_custom_html": "<main>Before clear</main>",
			"sanitization_report": map[string]any{
				"removed_tags":       []any{},
				"removed_attributes": []any{},
				"total_removed":      0,
				"truncated":          false,
			},
		})
	})

	cmd := testutil.Command(newPageClearCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"prod1"})
	testutil.MustExecute(t, cmd)

	snapshots := findSnapshotFiles(t, home)
	if len(snapshots) != 1 {
		t.Fatalf("got %d snapshots, want 1", len(snapshots))
	}
	data, err := os.ReadFile(snapshots[0])
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(data) != "<main>Before clear</main>" {
		t.Fatalf("snapshot = %q", data)
	}
}
