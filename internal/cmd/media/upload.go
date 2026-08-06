package media

import (
	"crypto/md5" // #nosec G501 -- S3 requires an MD5 Content-MD5 header for direct uploads; not used for security.
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

// maxMediaImageBytes mirrors the server's media-library cap
// (CreatePublicMediaService::MAX_IMAGE_BYTES): images render inline on pages,
// so the pipeline only accepts small files.
const maxMediaImageBytes = 10 * 1024 * 1024

const maxDirectUploadErrorBody = 4 * 1024

type plannedMediaUpload struct {
	Path        string
	Filename    string
	ContentType string
	Checksum    string
	Size        int64
}

type directUploadResponse struct {
	SignedID     string `json:"signed_id"`
	DirectUpload struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"direct_upload"`
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

Only images are accepted (JPEG, PNG, GIF, WebP, BMP, or ICO — matching the
server, which takes any image format except SVG), up to 10 MB. The image is
content-moderated during upload and rejected if flagged.`,
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad media upload ./logo.png
  gumroad media upload ./logo.png --name "Store logo"
  gumroad media upload ./logo.png --json --jq '.media.url'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			plan, err := describeMediaUpload(args[0])
			if err != nil {
				return err
			}

			if opts.DryRun {
				return renderUploadDryRun(opts, plan)
			}

			return runMediaUpload(opts, plan, name)
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

	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(sample[:n]), ";")[0]))
	if detected == "image/svg+xml" || strings.HasSuffix(strings.ToLower(filepath.Ext(path)), ".svg") {
		return "", cmdutil.InvalidInputErrorf("SVG images are not supported in the media library; use JPEG, PNG, GIF, WebP, BMP, or ICO")
	}
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

	signedID, err := reserveDirectUpload(client, plan)
	if err != nil {
		return err
	}
	if err := putDirectUpload(opts, plan, signedID.DirectUpload.URL, signedID.DirectUpload.Headers); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("signed_blob_id", signedID.SignedID)
	if name != "" {
		params.Set("name", name)
	}
	data, err := client.Post("/media", params)
	if err != nil {
		return err
	}

	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	resp, err := cmdutil.DecodeJSON[mediaUploadResponse](data)
	if err != nil {
		return err
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

func reserveDirectUpload(client *api.Client, plan plannedMediaUpload) (directUploadResponse, error) {
	params := url.Values{}
	params.Set("purpose", "media")
	params.Set("blob[filename]", plan.Filename)
	params.Set("blob[byte_size]", strconv.FormatInt(plan.Size, 10))
	params.Set("blob[checksum]", plan.Checksum)
	params.Set("blob[content_type]", plan.ContentType)

	data, err := client.Post("/direct_uploads", params)
	if err != nil {
		return directUploadResponse{}, err
	}
	resp, err := cmdutil.DecodeJSON[directUploadResponse](data)
	if err != nil {
		return directUploadResponse{}, err
	}
	if resp.SignedID == "" {
		return directUploadResponse{}, fmt.Errorf("direct upload response did not include signed_id")
	}
	if resp.DirectUpload.URL == "" {
		return directUploadResponse{}, fmt.Errorf("direct upload response did not include upload URL")
	}
	return resp, nil
}

// s3HTTPClientForTesting redirects the direct-upload PUT at a test server.
// Production leaves this nil so the default client is used. Tests in this
// package must not use t.Parallel — overwriting this var across goroutines
// would race.
var s3HTTPClientForTesting *http.Client

func putDirectUpload(opts cmdutil.Options, plan plannedMediaUpload, uploadURL string, headers map[string]string) error {
	file, err := os.Open(plan.Path)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", plan.Path, err)
	}
	defer func() { _ = file.Close() }()

	req, err := http.NewRequestWithContext(opts.Context, http.MethodPut, uploadURL, file)
	if err != nil {
		return fmt.Errorf("could not create direct upload request: %w", err)
	}
	req.ContentLength = plan.Size
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", plan.ContentType)
	}
	if req.Header.Get("Content-MD5") == "" {
		req.Header.Set("Content-MD5", plan.Checksum)
	}

	httpClient := http.DefaultClient
	if s3HTTPClientForTesting != nil {
		httpClient = s3HTTPClientForTesting
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("direct upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDirectUploadErrorBody))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("direct upload failed with HTTP %d: %s", resp.StatusCode, message)
	}
	return nil
}

type dryRunUploadPlan struct {
	DryRun      bool   `json:"dry_run"`
	Action      string `json:"action"`
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

func renderUploadDryRun(opts cmdutil.Options, plan plannedMediaUpload) error {
	payload := dryRunUploadPlan{
		DryRun:      true,
		Action:      "media upload",
		Path:        plan.Path,
		Filename:    plan.Filename,
		ContentType: plan.ContentType,
		Size:        plan.Size,
	}
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			"media upload",
			plan.Path,
			plan.Filename,
			plan.ContentType,
			strconv.FormatInt(plan.Size, 10),
		}})
	}
	if opts.Quiet {
		return nil
	}
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": media upload "+plan.Path); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Filename: %s\n", plan.Filename); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Content type: %s (%d bytes)\n", plan.ContentType, plan.Size)
}
