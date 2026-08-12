package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type fileDownloadURLResponse struct {
	SignedURL    string      `json:"signed_url"`
	ExternalLink bool        `json:"external_link"`
	File         productFile `json:"file"`
}

func newFilesDownloadCmd() *cobra.Command {
	var outputPath string
	var force bool

	cmd := &cobra.Command{
		Use:   "download <product-id> <file-id>",
		Short: "Download a product file for content review",
		Long: "Download a product file's actual contents through the admin API, so content review (malware reports, piracy verification, TOS review) does not need a real purchase.\n\n" +
			"The file id comes from `gumroad admin products view <product-id>`, which lists every file including soft-deleted ones — deleted files can still be downloaded, because the content under review is often the removed prior version.\n\n" +
			"Files whose \"file\" is an external link have no stored bytes; the command prints the link instead of fetching a third-party host.\n\n" +
			"Every download is audit-logged server-side and requires per-actor admin auth.",
		Args: cmdutil.ExactArgs(2),
		Example: `  gumroad admin products files download abc123 f_1
  gumroad admin products files download abc123 f_1 -o review.zip
  gumroad admin products files download abc123 f_1 -o - | shasum
  gumroad admin products files download abc123 f_1 --json`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID, fileID := args[0], args[1]

			// JSON output owns stdout, so it cannot be combined with streaming
			// the file bytes there.
			if outputPath == "-" && opts.UsesJSONOutput() {
				return cmdutil.InvalidInputErrorf("--json/--jq cannot be combined with -o - (both write to stdout); use -o <path> to keep the file")
			}
			if err := checkDownloadDestinationWritable(outputPath, force); err != nil {
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
					return cmdutil.InvalidInputErrorf("file %s is an external link, not a stored file — open it directly: %s", fileID, resp.SignedURL)
				}
				if resp.SignedURL == "" {
					return cmdutil.InvalidInputErrorf("the server did not return a download URL for file %s", fileID)
				}

				dest := downloadDestination(outputPath, resp.File, fileID)
				if err := downloadToFile(opts, resp.SignedURL, dest, force); err != nil {
					return err
				}
				if opts.UsesJSONOutput() {
					return cmdutil.PrintJSONResponse(opts, data)
				}
				return renderDownloadSuccess(opts, resp.File, dest)
			})
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Write the file to this path (- for stdout; defaults to the file's own name)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite the output file if it already exists")

	return cmd
}

// checkDownloadDestinationWritable fails fast when an explicit destination
// already exists and --force wasn't given, so no API call is wasted. The check
// runs again when the file is actually replaced (see downloadToFile) — this
// one is only an early exit, and the default destination is only known after
// the response arrives.
func checkDownloadDestinationWritable(dest string, force bool) error {
	if dest == "" || dest == "-" || force {
		return nil
	}
	if _, err := os.Stat(dest); err == nil {
		return downloadDestinationExistsError(dest)
	}
	return nil
}

func downloadDestinationExistsError(dest string) error {
	return cmdutil.InvalidInputErrorf("%s already exists (use --force to overwrite, or -o to pick another path)", dest)
}

// downloadDestination picks the implicit output path from the server-provided
// file name. The name is seller-controlled, so only its base name is used —
// a name like "../evil.pdf" must not write outside the working directory —
// and control characters are stripped so the name can't smuggle terminal
// escape sequences into the success output or the shell.
func downloadDestination(outputPath string, file productFile, fileID string) string {
	if outputPath != "" {
		return outputPath
	}
	name := strings.Map(func(r rune) rune {
		// unicode.IsControl covers C0, DEL, and the C1 range (U+0080–U+009F)
		// — U+009B is a single-rune CSI that would otherwise survive a
		// bytes-below-0x20 check and reach the terminal.
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(file.FileName))
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		return fileID
	}
	return name
}

// downloadToFile streams the signed URL to dest via a temporary file in the
// same directory, moving it into place only after the whole download has been
// written. The destination is never touched on a failed or partial write, and
// --force is enforced at the moment the file is actually replaced.
func downloadToFile(opts cmdutil.Options, signedURL, dest string, force bool) error {
	req, err := http.NewRequestWithContext(opts.Context, http.MethodGet, signedURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading the file failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading the file failed: storage returned HTTP %d", resp.StatusCode)
	}

	if dest == "-" {
		_, err := io.Copy(opts.Out(), resp.Body)
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".gumroad-download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// The download is a seller's product file pulled for review, not a
	// credential — match the world-readable mode the rest of the CLI uses for
	// non-secret content.
	if err := os.Chmod(tmpName, 0644); err != nil { //nolint:gosec // G302: reviewed product content
		os.Remove(tmpName)
		return err
	}
	if !force {
		if _, err := os.Stat(dest); err == nil {
			os.Remove(tmpName)
			return downloadDestinationExistsError(dest)
		}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
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
	if err := output.Writeln(opts.Out(), style.Bold(fmt.Sprintf("Downloaded %s → %s (%s)", output.EscapePlainField(name), output.EscapePlainField(dest), formatFileSize(int(file.FileSize))))); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Inspect it locally, e.g. `unzip -l %s` or your scanner of choice.\n", quotePathForShell(dest))
}

// quotePathForShell makes a path safe to copy-paste into the suggested
// follow-up command. Plain paths pass through untouched; anything with spaces
// or shell metacharacters gets single-quoted.
func quotePathForShell(path string) string {
	if plainDownloadPathPattern.MatchString(path) {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

var plainDownloadPathPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
