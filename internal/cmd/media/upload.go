package media

import (
	"crypto/md5" // #nosec G501 -- S3 requires an MD5 Content-MD5 header for direct uploads; not used for security.
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

// maxMediaImageBytes mirrors the server's media-library cap
// (CreatePublicMediaService::MAX_IMAGE_BYTES): images render inline on pages,
// so the pipeline only accepts small files.
const maxMediaImageBytes = 10 * 1024 * 1024

type plannedMediaUpload struct {
	Path        string
	Filename    string
	ContentType string
	Checksum    string
	Size        int64
}

type mediaItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	FileSize int64  `json:"file_size"`
}

type mediaUploadResponse struct {
	Media mediaItem `json:"media"`
}

func newUploadCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "upload <path>",
		Short: "Upload an image to the public media library and print its URL",
		Long: `Upload an image to your public media library and print its public URL.

The returned URL is served from Gumroad's public CDN — the only host that
custom product landing pages and profile pages are allowed to display images
from — so it is safe to embed in page HTML pushed with ` + "`gumroad pages push`" + `.

The CLI detects JPEG, PNG, GIF, WebP, BMP, and ICO images up to 10 MB. It
rejects SVG and image formats it cannot identify locally. Gumroad checks the
image again and runs content moderation before it hosts the file.`,
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad media upload ./logo.png
  gumroad media upload ./logo.png --name "Store logo"
  gumroad media upload ./logo.png --json --jq '.media.url'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := output.ValidateJQExpression(opts.JQExpr); err != nil {
				return err
			}

			plan, err := describeMediaUpload(args[0])
			if err != nil {
				return err
			}
			displayName := name
			if displayName == "" {
				displayName = plan.Filename
			}

			if opts.DryRun {
				return renderUploadDryRun(opts, plan, displayName)
			}

			return runMediaUpload(opts, plan, displayName)
		},
	}
	c.Flags().StringVar(&name, "name", "", "Display name for the file (defaults to the filename)")
	return c
}

func describeMediaUpload(path string) (plannedMediaUpload, error) {
	file, err := os.Open(path)
	if err != nil {
		return plannedMediaUpload{}, fmt.Errorf("could not open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return plannedMediaUpload{}, fmt.Errorf("could not stat %s: %w", path, err)
	}
	if info.IsDir() {
		return plannedMediaUpload{}, cmdutil.InvalidInputErrorf("%s is a directory", path)
	}
	if !info.Mode().IsRegular() {
		return plannedMediaUpload{}, cmdutil.InvalidInputErrorf("%s is not a regular file", path)
	}
	if info.Size() == 0 {
		return plannedMediaUpload{}, cmdutil.InvalidInputErrorf("%s is empty", path)
	}
	if info.Size() > maxMediaImageBytes {
		return plannedMediaUpload{}, cmdutil.InvalidInputErrorf("%s is %d bytes; media library images are capped at 10 MB", path, info.Size())
	}

	contentType, err := detectMediaImageContentType(path, file)
	if err != nil {
		return plannedMediaUpload{}, err
	}
	checksum, err := checksumFileMD5(file)
	if err != nil {
		return plannedMediaUpload{}, fmt.Errorf("could not checksum %s: %w", path, err)
	}

	return plannedMediaUpload{
		Path:        path,
		Filename:    filepath.Base(path),
		ContentType: contentType,
		Checksum:    checksum,
		Size:        info.Size(),
	}, nil
}

func detectMediaImageContentType(path string, file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	var sample [512]byte
	n, err := file.Read(sample[:])
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	// SVG can't be caught by sniffing — http.DetectContentType reports XML/plain
	// text for SVG bytes — so the guard is extension-based. A renamed SVG still
	// fails the image/ prefix check below, and the server rejects SVG regardless.
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return "", cmdutil.InvalidInputErrorf("SVG images are not supported in the media library; use JPEG, PNG, GIF, WebP, BMP, or ICO")
	}
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(sample[:n]), ";")[0]))
	if !strings.HasPrefix(detected, "image/") {
		return "", cmdutil.InvalidInputErrorf("%s is not an image; the media library accepts JPEG, PNG, GIF, WebP, BMP, or ICO images", path)
	}
	return detected, nil
}

func checksumFileMD5(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := md5.New() // #nosec G401 -- S3 Content-MD5 integrity header, not a security use.
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}

func runMediaUpload(opts cmdutil.Options, plan plannedMediaUpload, name string) error {
	token, err := config.Token()
	if err != nil {
		return err
	}
	client := cmdutil.NewAPIClient(opts, token)
	var sp *output.Spinner
	if cmdutil.ShouldShowSpinner(opts) {
		sp = output.NewSpinnerTo("Reserving upload for "+plan.Filename+"...", opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	signedID, err := reserveDirectUpload(client, plan)
	if err != nil {
		return err
	}
	if sp != nil {
		sp.SetMessage("Uploading " + plan.Filename + " (" + uploadui.HumanBytes(plan.Size) + ")...")
	}
	if err := putDirectUpload(opts, plan, signedID.DirectUpload.URL, signedID.DirectUpload.Headers); err != nil {
		return mediaDirectUploadError(err, signedID, plan, name)
	}

	params := url.Values{}
	params.Set("signed_blob_id", signedID.SignedID)
	if name != "" {
		params.Set("name", name)
	}
	if sp != nil {
		sp.SetMessage("Finalizing and moderating " + plan.Filename + "...")
	}
	data, err := client.Post("/media", params)
	if err != nil {
		return mediaCommitError(err, signedID, plan, name)
	}
	if sp != nil {
		sp.Stop()
	}
	resp, err := validateMediaUploadResponse(data)
	if err != nil {
		return mediaCommitResponseError(err, signedID, plan, name)
	}

	if err := renderMediaUpload(opts, data, resp, plan); err != nil {
		return mediaOutputError(err, resp.Media)
	}
	return nil
}

func renderMediaUpload(opts cmdutil.Options, data []byte, resp mediaUploadResponse, plan plannedMediaUpload) error {
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{resp.Media.ID, resp.Media.URL}})
	}
	if opts.Quiet {
		return nil
	}
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green("Uploaded")+" "+plan.Filename+" ("+resp.Media.ID+")"); err != nil {
		return err
	}
	if err := output.Writeln(opts.Out(), resp.Media.URL); err != nil {
		return err
	}
	return output.Writeln(opts.Out(), "Embed this URL in your page HTML, then publish with `gumroad pages push`.")
}

func validateMediaUploadResponse(data []byte) (mediaUploadResponse, error) {
	resp, err := cmdutil.DecodeJSON[mediaUploadResponse](data)
	if err != nil {
		return mediaUploadResponse{}, err
	}
	if resp.Media.ID == "" {
		return mediaUploadResponse{}, fmt.Errorf("media response did not include media.id")
	}
	if resp.Media.URL == "" {
		return mediaUploadResponse{}, fmt.Errorf("media response did not include media.url")
	}
	return resp, nil
}
