package games

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// zipDir writes the contents of dir to a temporary zip with paths relative to dir, so the
// directory's index.html sits at the archive root. The caller removes the returned file.
func zipDir(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return "", fmt.Errorf("%s has no index.html at its top level; point deploy at the folder that contains it", dir)
	}

	out, err := os.CreateTemp("", "gumroad-game-*.zip")
	if err != nil {
		return "", fmt.Errorf("could not create temporary zip: %w", err)
	}
	name := out.Name()
	fail := func(err error) (string, error) {
		_ = out.Close()
		_ = os.Remove(name)
		return "", err
	}

	zw := zip.NewWriter(out)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.") {
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(w, f)
		return err
	})
	if walkErr != nil {
		return fail(fmt.Errorf("could not zip %s: %w", dir, walkErr))
	}
	if err := zw.Close(); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}
