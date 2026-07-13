package products

import (
	"crypto/md5" // #nosec G501
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func collectProductMedia(coverImage string, previewImages []string, previewVideos []string, thumbnail string) []requestedProductMedia {
	var media []requestedProductMedia
	if coverImage != "" {
		media = append(media, requestedProductMedia{Kind: productMediaCover, Path: coverImage})
	}
	for _, path := range previewImages {
		media = append(media, requestedProductMedia{Kind: productMediaPreview, Path: path})
	}
	for _, path := range previewVideos {
		media = append(media, requestedProductMedia{Kind: productMediaPreview, Path: path, IsVideo: true})
	}
	if thumbnail != "" {
		media = append(media, requestedProductMedia{Kind: productMediaThumbnail, Path: thumbnail})
	}
	return media
}

func validateProductMediaFlagPaths(cmd *cobra.Command, coverImage string, previewImages []string, previewVideos []string, thumbnail string) error {
	if cmd.Flags().Changed("cover-image") && strings.TrimSpace(coverImage) == "" {
		return cmdutil.UsageErrorf(cmd, "--cover-image cannot be empty")
	}
	if cmd.Flags().Changed("thumbnail") && strings.TrimSpace(thumbnail) == "" {
		return cmdutil.UsageErrorf(cmd, "--thumbnail cannot be empty")
	}
	for _, path := range previewImages {
		if strings.TrimSpace(path) == "" {
			return cmdutil.UsageErrorf(cmd, "--preview-image cannot be empty")
		}
	}
	for _, path := range previewVideos {
		if strings.TrimSpace(path) == "" {
			return cmdutil.UsageErrorf(cmd, "--preview-video cannot be empty")
		}
	}
	return nil
}

func describeProductMedia(media []requestedProductMedia) ([]plannedProductMedia, error) {
	planned := make([]plannedProductMedia, len(media))
	for i, current := range media {
		plan, err := describeSingleProductMedia(current)
		if err != nil {
			return nil, err
		}
		planned[i] = plan
	}
	return planned, nil
}

func describeSingleProductMedia(media requestedProductMedia) (plannedProductMedia, error) {
	noun := productMediaNoun(media)
	file, err := os.Open(media.Path)
	if err != nil {
		return plannedProductMedia{}, fmt.Errorf("could not open %s: %w", noun, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return plannedProductMedia{}, fmt.Errorf("could not stat %s: %w", noun, err)
	}
	if info.IsDir() {
		return plannedProductMedia{}, fmt.Errorf("%s is a directory", media.Path)
	}
	if !info.Mode().IsRegular() {
		return plannedProductMedia{}, fmt.Errorf("%s is not a regular file", media.Path)
	}
	if info.Size() == 0 {
		return plannedProductMedia{}, fmt.Errorf("%s is empty", media.Path)
	}
	if media.IsVideo {
		if info.Size() > uploadMaxProductVideoFileSize() {
			return plannedProductMedia{}, fmt.Errorf("%s size %d bytes exceeds maximum of %d bytes (500 MB)", media.Path, info.Size(), uploadMaxProductVideoFileSize())
		}
	} else if info.Size() > uploadMaxProductMediaFileSize() {
		return plannedProductMedia{}, fmt.Errorf("%s size %d bytes exceeds maximum of %d bytes (50 MB)", media.Path, info.Size(), uploadMaxProductMediaFileSize())
	}

	var contentType string
	if media.IsVideo {
		contentType, err = detectProductVideoContentType(media.Path, file)
	} else {
		contentType, err = detectProductImageContentType(media.Path, file)
	}
	if err != nil {
		return plannedProductMedia{}, err
	}
	checksum, err := checksumFileMD5(file)
	if err != nil {
		return plannedProductMedia{}, fmt.Errorf("could not checksum %s: %w", noun, err)
	}

	return plannedProductMedia{
		requestedProductMedia: media,
		Filename:              filepath.Base(media.Path),
		ContentType:           contentType,
		Checksum:              checksum,
		Size:                  info.Size(),
	}, nil
}

func uploadMaxProductMediaFileSize() int64 {
	return 50 * 1024 * 1024
}

func uploadMaxProductVideoFileSize() int64 {
	return 500 * 1024 * 1024
}

func productMediaNoun(media requestedProductMedia) string {
	if media.IsVideo {
		return string(media.Kind) + " video"
	}
	return string(media.Kind) + " image"
}

func detectProductImageContentType(path string, file *os.File) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".webp" {
		return "", fmt.Errorf("WebP images are not supported for product media; use JPEG, PNG, or GIF")
	}

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

	if isWebPImage(sample[:n]) {
		return "", fmt.Errorf("WebP images are not supported for product media; use JPEG, PNG, or GIF")
	}
	rawDetected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(sample[:n]), ";")[0]))
	if rawDetected == "image/webp" {
		return "", fmt.Errorf("WebP images are not supported for product media; use JPEG, PNG, or GIF")
	}
	detected := normalizeImageContentType(rawDetected)
	if detected != "" {
		return detected, nil
	}

	return "", fmt.Errorf("unsupported product media type for %s; use a JPEG, PNG, or GIF image", path)
}

func isWebPImage(sample []byte) bool {
	return len(sample) >= 12 &&
		string(sample[0:4]) == "RIFF" &&
		string(sample[8:12]) == "WEBP"
}

func detectProductVideoContentType(path string, file *os.File) (string, error) {
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

	rawDetected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(sample[:n]), ";")[0]))
	detected := normalizeVideoContentType(rawDetected)
	if detected != "" {
		return detected, nil
	}
	// The extension only decides the content type when sniffing the file's
	// bytes was inconclusive. If the bytes positively identify something that
	// is not a video (an image, an archive, ...), a video extension on the
	// filename must not override that: the server trusts the content type the
	// CLI declares, so a mislabeled file would become a broken cover.
	if sniffWasInconclusive(rawDetected) {
		if fromExtension := videoContentTypeForExtension(strings.ToLower(filepath.Ext(path))); fromExtension != "" {
			return fromExtension, nil
		}
	}

	return "", fmt.Errorf("unsupported preview video type for %s; use an MP4, MOV, M4V, MPEG, WMV, or WebM video", path)
}

func sniffWasInconclusive(rawDetected string) bool {
	// http.DetectContentType answers application/octet-stream when it cannot
	// identify the bytes at all, and it only knows a few video containers, so
	// an unrecognized video/* answer is also inconclusive. Anything else
	// (image/*, application/zip, text/plain, ...) is a positive match for a
	// non-video file and must not be overridden by the filename's extension.
	return rawDetected == "application/octet-stream" || strings.HasPrefix(rawDetected, "video/")
}

func normalizeVideoContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "video/mp4", "video/quicktime", "video/mpeg", "video/webm", "video/x-m4v", "video/x-ms-wmv":
		return contentType
	default:
		return ""
	}
}

func videoContentTypeForExtension(ext string) string {
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".m4v":
		return "video/x-m4v"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".webm":
		return "video/webm"
	default:
		return ""
	}
}

func normalizeImageContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/gif":
		return "image/gif"
	default:
		return ""
	}
}

func checksumFileMD5(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := md5.New() // #nosec G401
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}
