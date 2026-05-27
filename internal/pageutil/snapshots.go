package pageutil

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

const (
	maxSnapshots   = 10
	snapshotPrefix = "previous-"
	snapshotSuffix = ".html"
	snapshotLayout = "2006-01-02T15-04-05.000000000Z"
)

type Snapshot struct {
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Path      string    `json:"path"`
	Size      int64     `json:"size_bytes"`
}

type SavedSnapshot struct {
	Written  bool   `json:"written"`
	Path     string `json:"path,omitempty"`
	Size     int64  `json:"size_bytes,omitempty"`
	Skipped  bool   `json:"skipped,omitempty"`
	SkipKind string `json:"skip_kind,omitempty"`
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func SaveSnapshot(resource string, id string, html *string) (SavedSnapshot, error) {
	if html == nil {
		return SavedSnapshot{Skipped: true, SkipKind: "empty_previous_html"}, nil
	}

	dir, err := snapshotDir(resource, id)
	if err != nil {
		return SavedSnapshot{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return SavedSnapshot{}, fmt.Errorf("could not create snapshot directory: %w", err)
	}

	path := filepath.Join(dir, snapshotPrefix+nowUTC().Format(snapshotLayout)+snapshotSuffix)
	if err := os.WriteFile(path, []byte(*html), 0600); err != nil {
		return SavedSnapshot{}, fmt.Errorf("could not write snapshot: %w", err)
	}
	if err := PruneSnapshots(resource, id, maxSnapshots); err != nil {
		return SavedSnapshot{}, err
	}
	return SavedSnapshot{Written: true, Path: path, Size: int64(len([]byte(*html)))}, nil
}

func ListSnapshots(resource string, id string) ([]Snapshot, error) {
	dir, err := snapshotDir(resource, id)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read snapshots: %w", err)
	}

	var snapshots []Snapshot
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ts, ok := parseSnapshotTimestamp(entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("could not stat snapshot: %w", err)
		}
		snapshots = append(snapshots, Snapshot{
			Timestamp: ts,
			Path:      filepath.Join(dir, entry.Name()),
			Size:      info.Size(),
		})
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})
	for i := range snapshots {
		snapshots[i].Index = i + 1
	}
	return snapshots, nil
}

func ReadSnapshot(resource string, id string, index int) (Snapshot, string, error) {
	if index < 1 {
		return Snapshot{}, "", fmt.Errorf("--snapshot must be 1 or greater")
	}
	snapshots, err := ListSnapshots(resource, id)
	if err != nil {
		return Snapshot{}, "", err
	}
	if len(snapshots) == 0 {
		return Snapshot{}, "", fmt.Errorf("no snapshots found for %s/%s", resource, id)
	}
	if index > len(snapshots) {
		return Snapshot{}, "", fmt.Errorf("snapshot %d not found; run `gumroad products page history %s`", index, id)
	}

	snapshot := snapshots[index-1]
	data, err := os.ReadFile(snapshot.Path)
	if err != nil {
		return Snapshot{}, "", fmt.Errorf("could not read snapshot: %w", err)
	}
	return snapshot, string(data), nil
}

func PruneSnapshots(resource string, id string, keep int) error {
	if keep < 1 {
		return nil
	}
	snapshots, err := ListSnapshots(resource, id)
	if err != nil {
		return err
	}
	if len(snapshots) <= keep {
		return nil
	}
	for _, snapshot := range snapshots[keep:] {
		if err := os.Remove(snapshot.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not prune snapshot: %w", err)
		}
	}
	return nil
}

func RenderHistory(opts cmdutil.Options, resource string, id string, snapshots []Snapshot) error {
	if opts.UsesJSONOutput() {
		if snapshots == nil {
			snapshots = []Snapshot{}
		}
		payload := struct {
			Success   bool       `json:"success"`
			Resource  string     `json:"resource"`
			ID        string     `json:"id,omitempty"`
			Snapshots []Snapshot `json:"snapshots"`
		}{Success: true, Resource: resource, ID: id, Snapshots: snapshots}
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode history: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}

	if opts.PlainOutput {
		rows := make([][]string, 0, len(snapshots))
		for _, snapshot := range snapshots {
			rows = append(rows, []string{
				fmt.Sprintf("%d", snapshot.Index),
				snapshot.Timestamp.Format(time.RFC3339),
				fmt.Sprintf("%d", snapshot.Size),
				snapshot.Path,
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	if len(snapshots) == 0 {
		return cmdutil.PrintInfo(opts, "No snapshots found for "+resource+"/"+id+".")
	}

	tbl := output.NewStyledTable(opts.Style(), "INDEX", "TIMESTAMP", "BYTES", "PATH")
	for _, snapshot := range snapshots {
		tbl.AddRow(
			fmt.Sprintf("%d", snapshot.Index),
			snapshot.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", snapshot.Size),
			snapshot.Path,
		)
	}
	return tbl.Render(opts.Out())
}

func snapshotDir(resource string, id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %w", err)
	}
	parts := []string{home, ".gumroad", "pages", url.PathEscape(resource)}
	if strings.TrimSpace(id) != "" {
		parts = append(parts, url.PathEscape(id))
	}
	return filepath.Join(parts...), nil
}

func parseSnapshotTimestamp(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, snapshotPrefix) || !strings.HasSuffix(name, snapshotSuffix) {
		return time.Time{}, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(name, snapshotPrefix), snapshotSuffix)
	ts, err := time.Parse(snapshotLayout, value)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}
