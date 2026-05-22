package products

import (
	"crypto/md5" // #nosec G501
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func collectProductMedia(coverImage string, previewImages []string, thumbnail string) []requestedProductMedia {
	var media []requestedProductMedia
	if coverImage != "" {
		media = append(media, requestedProductMedia{Kind: productMediaCover, Path: coverImage})
	}
	for _, path := range previewImages {
		media = append(media, requestedProductMedia{Kind: productMediaPreview, Path: path})
	}
	if thumbnail != "" {
		media = append(media, requestedProductMedia{Kind: productMediaThumbnail, Path: thumbnail})
	}
	return media
}

func validateProductMediaFlagPaths(cmd *cobra.Command, coverImage string, previewImages []string, thumbnail string) error {
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
	file, err := os.Open(media.Path)
	if err != nil {
		return plannedProductMedia{}, fmt.Errorf("could not open %s image: %w", media.Kind, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return plannedProductMedia{}, fmt.Errorf("could not stat %s image: %w", media.Kind, err)
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
	if info.Size() > uploadMaxProductMediaFileSize() {
		return plannedProductMedia{}, fmt.Errorf("%s size %d bytes exceeds maximum of %d bytes (50 MB)", media.Path, info.Size(), uploadMaxProductMediaFileSize())
	}

	contentType, err := detectProductImageContentType(media.Path, file)
	if err != nil {
		return plannedProductMedia{}, err
	}
	checksum, err := checksumFileMD5(file)
	if err != nil {
		return plannedProductMedia{}, fmt.Errorf("could not checksum %s image: %w", media.Kind, err)
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

	if byExt := normalizeImageContentType(mime.TypeByExtension(ext)); byExt != "" {
		return byExt, nil
	}

	return "", fmt.Errorf("unsupported product media type for %s; use a JPEG, PNG, or GIF image", path)
}

func isWebPImage(sample []byte) bool {
	return len(sample) >= 12 &&
		string(sample[0:4]) == "RIFF" &&
		string(sample[8:12]) == "WEBP"
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
