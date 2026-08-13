//go:build (darwin && !ios) || (linux && !android)

package products

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func validateFileDownloadSupport(string) error { return nil }
func lockPrivateDirectory(path string, _ bool) (*os.File, string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	lockedPath := path
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, "", err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		trusted := ok && (fmt.Sprint(stat.Uid) == fmt.Sprint(os.Geteuid()) || stat.Uid == 0)
		if !trusted || info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return nil, "", fmt.Errorf("refusing private download below replaceable directory %s", path)
		}
		if err := validatePrivateAncestor(path); err != nil {
			return nil, "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	fd, err := unix.Open(lockedPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(fd), lockedPath)
	fileInfo, err := file.Stat()
	pathInfo, pathErr := os.Lstat(lockedPath)
	if err != nil || pathErr != nil || !os.SameFile(fileInfo, pathInfo) {
		file.Close()
		return nil, "", fmt.Errorf("private download directory changed during validation")
	}
	return file, lockedPath, nil
}
func createPrivateFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
}
