package products

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageHistoryPlainListsSnapshots(t *testing.T) {
	setSnapshotHome(t)
	html := "<main>Old</main>"
	if _, err := pageutil.SaveSnapshot("products", "prod1", &html); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	cmd := testutil.Command(newPageHistoryCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "1\t") || !strings.Contains(out, ".html") {
		t.Fatalf("unexpected history output: %q", out)
	}
}

func TestPageHistoryJSONListsSnapshots(t *testing.T) {
	setSnapshotHome(t)
	html := "<main>Old</main>"
	if _, err := pageutil.SaveSnapshot("products", "prod1", &html); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	cmd := testutil.Command(newPageHistoryCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var body struct {
		Success   bool  `json:"success"`
		Snapshots []any `json:"snapshots"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("history JSON invalid: %v\n%s", err, out)
	}
	if !body.Success || len(body.Snapshots) != 1 {
		t.Fatalf("unexpected history JSON: %q", out)
	}
}

func TestPageHistoryJSONEmptySnapshotsUsesArray(t *testing.T) {
	setSnapshotHome(t)

	cmd := testutil.Command(newPageHistoryCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"prod1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var body struct {
		Success   bool              `json:"success"`
		Snapshots []json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("history JSON invalid: %v\n%s", err, out)
	}
	if !body.Success {
		t.Fatalf("expected success response: %q", out)
	}
	if body.Snapshots == nil {
		t.Fatalf("snapshots should be an empty array, not null: %q", out)
	}
	if len(body.Snapshots) != 0 {
		t.Fatalf("got %d snapshots, want 0: %q", len(body.Snapshots), out)
	}
}
