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
//
// dir itself may be a symlink, since the user named it. Links inside it are never followed:
// following one could pack files from anywhere on disk, and skipping it would ship a build
// that is silently missing a file, so the walk fails on them instead.
func zipDir(dir string) (string, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("could not resolve %s: %w", dir, err)
	}
	index, err := os.Lstat(filepath.Join(root, "index.html"))
	if err != nil {
		return "", fmt.Errorf("%s has no index.html at its top level; point deploy at the folder that contains it", dir)
	}
	if !index.Mode().IsRegular() {
		return "", fmt.Errorf("%s: index.html must be a regular file, not a link or folder", dir)
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
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file or folder; links are not followed, so copy in the file it points to", rel)
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
