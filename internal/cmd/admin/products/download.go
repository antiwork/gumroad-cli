package products

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type fileDownloadURLResponse struct {
	SignedURL    string          `json:"signed_url"`
	ExternalLink bool            `json:"external_link"`
	File         json.RawMessage `json:"file"`
}
type fileDownloadOutput struct {
	Success bool            `json:"success"`
	Path    string          `json:"path"`
	File    json.RawMessage `json:"file"`
}

const reviewDirectoryName, maxReviewFileIDLength = "gumroad-review", 80

func newFilesDownloadCmd() *cobra.Command {
	var outputPath string
	var force bool
	cmd := &cobra.Command{
		Use:   "download <product-id> <file-id>",
		Short: "Download a product file for content review",
		Long:  "Download a product file's actual contents through the admin API, so content review does not need a real purchase.\n\nGet file ids with `gumroad admin products view <product-id> --json --jq '.product.files[] | [.id, .display_name]'`. The result includes soft-deleted files, which can still be downloaded because reviews often concern a removed prior version.\n\nExternal links have no stored bytes, so the command prints the link without fetching its host.\n\nJSON output reports the path and file metadata without exposing the temporary signed URL.\n\nEvery download is audit-logged and requires per-actor admin auth.",
		Args:  cmdutil.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID, fileID := args[0], args[1]
			if err := output.ValidateJQExpression(opts.JQExpr); err != nil {
				return err
			}
			fetchOpts := opts
			fetchOpts.JSONOutput = false
			fetchOpts.JQExpr = ""
			path := cmdutil.JoinPath("products", productID, "files", fileID, "download_url")
			return admincmd.Run(fetchOpts, "Fetching download URL...", func(client *adminapi.Client) (json.RawMessage, error) {
				return client.Get(path, url.Values{})
			}, func(data json.RawMessage) error {
				resp, err := cmdutil.DecodeJSON[fileDownloadURLResponse](data)
				if err != nil {
					return err
				}
				if resp.ExternalLink {
					return cmdutil.InvalidInputErrorf("file %s is an external link, not a stored file — open it directly: %s", output.EscapePlainField(fileID), output.EscapePlainField(resp.SignedURL))
				}
				if outputPath == "-" && opts.UsesJSONOutput() {
					return cmdutil.InvalidInputErrorf("--json/--jq cannot be combined with -o - (both write to stdout); use -o <path> to keep the file")
				}
				if err := validateFileDownloadSupport(outputPath); err != nil {
					return err
				}
				if err := checkDownloadDestinationWritable(outputPath, force); err != nil {
					return err
				}
				if resp.SignedURL == "" {
					return cmdutil.InvalidInputErrorf("the server did not return a download URL for file %s", fileID)
				}
				file, err := cmdutil.DecodeJSON[productFile](resp.File)
				if err != nil {
					return err
				}
				dest, err := downloadDestination(outputPath, fileID)
				if err != nil {
					return err
				}
				if err := checkDownloadDestinationWritable(dest, force); err != nil {
					return err
				}
				var stagingDir string
				installDest := dest
				var stagedDownload *os.File
				var stagingDirLock io.Closer
				if dest != "-" {
					stagingDir, stagingDirLock, stagedDownload, err = prepareDownloadStaging(dest)
					if err != nil {
						return err
					}
					installDest = filepath.Join(filepath.Dir(stagingDir), filepath.Base(dest))
					defer func() {
						stagedDownload.Close()
						os.Remove(stagedDownload.Name())
						stagingDirLock.Close()
						os.Remove(stagingDir)
					}()
				}
				machineOutput, err := stageDownloadOutput(opts, resp.File, stagingDir, dest)
				if err != nil {
					return err
				}
				if machineOutput != nil {
					defer func() {
						machineOutput.Close()
						os.Remove(machineOutput.Name())
					}()
				}
				if err := downloadToFile(opts, resp.SignedURL, stagedDownload, installDest, force); err != nil {
					return err
				}
				if machineOutput != nil {
					_, err := io.Copy(opts.Out(), machineOutput)
					return err
				}
				return renderDownloadSuccess(opts, file, dest)
			})
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Write the file to this path (- for stdout; defaults to a private gumroad-review path)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite the output file if it already exists")
	return cmd
}
func checkDownloadDestinationWritable(dest string, force bool) error {
	if dest == "" || dest == "-" {
		return nil
	}
	if os.IsPathSeparator(dest[len(dest)-1]) {
		return cmdutil.InvalidInputErrorf("%s is a directory path; use -o with a file path", dest)
	}
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return cmdutil.InvalidInputErrorf("%s is a directory; use -o with a file path", dest)
	} else if err == nil && !force {
		return downloadDestinationExistsError(dest)
	}
	return nil
}
func downloadDestinationExistsError(dest string) error {
	return cmdutil.InvalidInputErrorf("%s already exists (use --force to overwrite, or -o to pick another path)", dest)
}
func downloadDestination(outputPath, fileID string) (string, error) {
	if outputPath != "" {
		return outputPath, nil
	}
	if err := preparePrivateDirectory(reviewDirectoryName); err != nil {
		return "", err
	}
	return filepath.Join(reviewDirectoryName, trustedReviewFilename(fileID)), nil
}
func prepareDownloadStaging(dest string) (string, io.Closer, *os.File, error) {
	parentLock, parent, err := lockPrivateDirectory(filepath.Dir(dest), false)
	if err != nil {
		return "", nil, nil, err
	}
	defer parentLock.Close()
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", nil, nil, err
	}
	dir := filepath.Join(parent, fmt.Sprintf(".gumroad-download-%x", suffix))
	if err := createPrivateDirectory(dir); err != nil {
		return "", nil, nil, err
	}
	lock, lockedDir, err := lockPrivateDirectory(dir, true)
	if err != nil {
		os.Remove(dir)
		return "", nil, nil, err
	}
	dir = lockedDir
	if err := makeOpenPathPrivate(lock, 0o700); err != nil {
		lock.Close()
		os.Remove(dir)
		return "", nil, nil, err
	}
	staged, err := createPrivateFile(filepath.Join(dir, "download"))
	if err != nil {
		lock.Close()
		os.Remove(dir)
		return "", nil, nil, err
	}
	if err := makeOpenPathPrivate(staged, 0o600); err != nil {
		staged.Close()
		os.Remove(staged.Name())
		lock.Close()
		os.Remove(dir)
		return "", nil, nil, err
	}
	return dir, lock, staged, nil
}
func preparePrivateDirectory(path string) error {
	parentLock, parent, err := lockPrivateDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer parentLock.Close()
	path = filepath.Join(parent, filepath.Base(path))
	if err := createPrivateDirectory(path); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return cmdutil.InvalidInputErrorf("%s exists but is not a directory", path)
	}
	lock, _, err := lockPrivateDirectory(path, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	return makeOpenPathPrivate(lock, 0o700)
}
func trustedReviewFilename(fileID string) string {
	var name strings.Builder
	for _, r := range fileID {
		if name.Len() >= maxReviewFileIDLength {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			name.WriteRune(r)
		} else {
			name.WriteByte('_')
		}
	}
	id := strings.Trim(name.String(), "_-")
	if id == "" {
		id = "unknown"
	}
	return "file-" + id + ".download"
}

var downloadStallTimeout, downloadHTTPClient = 2 * time.Minute, &http.Client{CheckRedirect: refuseDownloadRedirects}

func refuseDownloadRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
func downloadToFile(opts cmdutil.Options, signedURL string, staged *os.File, dest string, force bool) error {
	downloadURL, err := url.Parse(signedURL)
	if err != nil || downloadURL.Scheme != "https" || downloadURL.Host == "" || downloadURL.User != nil {
		return cmdutil.InvalidInputErrorf("the server returned an invalid secure download URL")
	}
	ctx, cancel := context.WithCancel(opts.Context)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return cmdutil.InvalidInputErrorf("the server returned an invalid secure download URL")
	}
	req.Header.Set("Accept-Encoding", "identity")
	stallTimer := time.AfterFunc(downloadStallTimeout, cancel)
	resp, err := downloadHTTPClient.Do(req)
	stallTimer.Stop()
	if err != nil {
		return stallAwareDownloadError(ctx, opts.Context, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading the file failed: storage returned HTTP %d", resp.StatusCode)
	}
	body := &stallTimeoutReader{r: resp.Body, cancel: cancel}
	if dest == "-" {
		if _, err := io.Copy(opts.Out(), body); err != nil {
			return stallAwareDownloadError(ctx, opts.Context, err)
		}
		return nil
	}
	if _, err := io.Copy(staged, body); err != nil {
		return stallAwareDownloadError(ctx, opts.Context, err)
	}
	return installDownloadedFile(staged, dest, force)
}
func installDownloadedFile(staged *os.File, dest string, force bool) error {
	if err := staged.Sync(); err != nil {
		return err
	}
	sourceInfo, err := staged.Stat()
	if err != nil {
		return err
	}
	tmpName := staged.Name()
	if err := staged.Close(); err != nil {
		return err
	}
	linked := false
	if !force {
		err = installNoReplace(tmpName, dest)
		if err != nil && !errors.Is(err, os.ErrExist) {
			err = os.Link(tmpName, dest)
			if err == nil {
				linked = true
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return downloadDestinationExistsError(dest)
			}
			return err
		}
	} else if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	destInfo, err := os.Stat(dest)
	if err != nil || !os.SameFile(sourceInfo, destInfo) {
		return fmt.Errorf("the installed download path changed during installation")
	}
	if linked {
		return os.Remove(tmpName)
	}
	return nil
}
func stallAwareDownloadError(ctx, parent context.Context, err error) error {
	if ctx.Err() != nil && parent.Err() == nil {
		return fmt.Errorf("downloading the file failed: storage sent no data for %s", downloadStallTimeout)
	}
	var urlErr *url.Error
	for errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
		urlErr = nil
	}
	return fmt.Errorf("downloading the file failed: %w", err)
}
func stageDownloadOutput(opts cmdutil.Options, file json.RawMessage, stagingDir, dest string) (*os.File, error) {
	if !opts.UsesJSONOutput() {
		return nil, nil
	}
	data, err := json.Marshal(fileDownloadOutput{
		Success: true,
		Path:    dest,
		File:    file,
	})
	if err != nil {
		return nil, fmt.Errorf("could not encode download output: %w", err)
	}
	staged, err := createPrivateFile(filepath.Join(stagingDir, "output"))
	if err != nil {
		return nil, err
	}
	if err := makeOpenPathPrivate(staged, 0o600); err != nil {
		staged.Close()
		os.Remove(staged.Name())
		return nil, err
	}
	stagedOpts := opts
	stagedOpts.Stdout = staged
	if err := cmdutil.PrintJSONResponse(stagedOpts, data); err != nil {
		staged.Close()
		os.Remove(staged.Name())
		return nil, err
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		staged.Close()
		os.Remove(staged.Name())
		return nil, err
	}
	return staged, nil
}

type stallTimeoutReader struct {
	r      io.Reader
	cancel context.CancelFunc
}

func (s *stallTimeoutReader) Read(p []byte) (int, error) {
	timer := time.AfterFunc(downloadStallTimeout, s.cancel)
	n, err := s.r.Read(p)
	timer.Stop()
	return n, err
}
func renderDownloadSuccess(opts cmdutil.Options, file productFile, dest string) error {
	if dest == "-" {
		return nil
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{file.ID, fileDisplayNameWithDeleted(file), dest, formatFileSize(int(file.FileSize))}})
	}
	if opts.Quiet {
		return nil
	}
	style := opts.Style()
	name := fileDisplayNameWithDeleted(file)
	if name == "" {
		name = file.ID
	}
	return output.Writeln(opts.Out(), style.Bold(fmt.Sprintf("Downloaded %s → %s (%s)", output.EscapePlainField(name), output.EscapePlainField(dest), formatFileSize(int(file.FileSize)))))
}
func verifyPrivateMode(file *os.File, mode os.FileMode) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Mode().Perm() != mode.Perm() {
		return fmt.Errorf("%s does not support private file permissions", file.Name())
	}
	return nil
}
