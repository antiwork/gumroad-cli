//go:build linux && !android

package products

import (
	"os"

	"golang.org/x/sys/unix"
)

func makeOpenPathPrivate(file *os.File, mode os.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		return err
	}
	return verifyPrivateMode(file, mode)
}
func createPrivateDirectory(path string) error { return os.Mkdir(path, 0o700) }
func installNoReplace(source, destination string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
}
func validatePrivateAncestor(string) error { return nil }
