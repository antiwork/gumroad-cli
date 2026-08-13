//go:build (!darwin && !linux) || ios || android

package products

import (
	"fmt"
	"os"
)

var errUnsupported = fmt.Errorf("local file downloads require macOS or Linux; use -o - to stream to stdout")

func validateFileDownloadSupport(path string) error {
	if path == "-" {
		return nil
	}
	return errUnsupported
}
func lockPrivateDirectory(string, bool) (*os.File, string, error) { return nil, "", errUnsupported }
func createPrivateFile(string) (*os.File, error)                  { return nil, errUnsupported }
func makeOpenPathPrivate(*os.File, os.FileMode) error             { return errUnsupported }
func createPrivateDirectory(string) error                         { return errUnsupported }
func installNoReplace(string, string) error                       { return errUnsupported }
