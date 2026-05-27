package pageutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveSnapshotListsNewestFirstAndPrunes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	base := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	i := 0
	oldNow := nowUTC
	nowUTC = func() time.Time {
		current := base.Add(time.Duration(i) * time.Second)
		i++
		return current
	}
	t.Cleanup(func() { nowUTC = oldNow })

	for n := 0; n < 12; n++ {
		html := string(rune('a' + n))
		if _, err := SaveSnapshot("products", "prod/with/slash", &html); err != nil {
			t.Fatalf("SaveSnapshot %d failed: %v", n, err)
		}
	}

	snapshots, err := ListSnapshots("products", "prod/with/slash")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snapshots) != 10 {
		t.Fatalf("got %d snapshots, want 10", len(snapshots))
	}
	if snapshots[0].Timestamp.Before(snapshots[1].Timestamp) {
		t.Fatalf("snapshots not newest first: %#v", snapshots[:2])
	}
	if filepath.Base(filepath.Dir(snapshots[0].Path)) != "prod%2Fwith%2Fslash" {
		t.Fatalf("snapshot ID was not path-escaped: %s", snapshots[0].Path)
	}
}

func TestReadSnapshotReturnsSelectedHTML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldNow := nowUTC
	nowUTC = func() time.Time { return time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { nowUTC = oldNow })

	html := "<main>old</main>"
	if _, err := SaveSnapshot("products", "prod1", &html); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	snapshot, got, err := ReadSnapshot("products", "prod1", 1)
	if err != nil {
		t.Fatalf("ReadSnapshot failed: %v", err)
	}
	if got != html {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(snapshot.Path); err != nil {
		t.Fatalf("snapshot path missing: %v", err)
	}
}
