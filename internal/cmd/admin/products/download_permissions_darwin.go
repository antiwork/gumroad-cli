//go:build darwin && !ios

package products

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

func makeOpenPathPrivate(file *os.File, mode os.FileMode) error {
	cmd := exec.Command("/bin/chmod", "-N", "/dev/fd/3")
	cmd.ExtraFiles = []*os.File{file}
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	return verifyPrivateMode(file, mode)
}
func createPrivateDirectory(path string) error { return os.Mkdir(path, 0o700) }
func installNoReplace(source, destination string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCL)
}
func validatePrivateAncestor(path string) error {
	out, err := exec.Command("/bin/ls", "-lde", path).Output()
	if err != nil {
		return err
	}
	if bytes.Contains(out, []byte(" allow ")) {
		return fmt.Errorf("refusing private download below ACL directory %s", path)
	}
	return nil
}
