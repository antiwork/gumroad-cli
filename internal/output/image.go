package output

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/term"
)

const (
	// Product images are decorative. Keep the budget tight so a slow image host
	// does not make the command feel sluggish.
	fetchTimeout = 750 * time.Millisecond
	maxBodySize  = 5 * 1024 * 1024 // 5MB
	maxDimension = 4096
)

// RenderImage downloads an image from imageURL and writes ANSI half-block
// pixel art to w, fitting within maxWidth terminal columns. Silent no-op on
// any failure — this is purely decorative.
func RenderImage(w io.Writer, imageURL string, maxWidth int) {
	RenderImageWithContext(context.Background(), w, imageURL, maxWidth)
}

// RenderImageWithContext downloads an image from imageURL and writes ANSI
// half-block pixel art to w, fitting within maxWidth terminal columns.
// Silent no-op on any failure — this is purely decorative.
func RenderImageWithContext(ctx context.Context, w io.Writer, imageURL string, maxWidth int) {
	if imageURL == "" || !NewStylerForWriter(w, false).Enabled() || maxWidth <= 0 {
		return
	}
	img, err := fetchAndDecode(ctx, imageURL)
	if err != nil {
		return
	}
	renderHalfBlock(w, img, maxWidth)
}

// ImageEnabled returns whether image rendering is enabled (follows color gate).
func ImageEnabled() bool {
	return NewStyler(false).Enabled()
}

// TerminalWidth returns the terminal width or defaultWidth if detection fails.
func TerminalWidth(defaultWidth int) int {
	return TerminalWidthFor(os.Stdout, defaultWidth)
}

// TerminalWidthFor returns the terminal width for the supplied writer when
// backed by a TTY file descriptor, or defaultWidth if detection fails.
func TerminalWidthFor(w io.Writer, defaultWidth int) int {
	if width, ok := terminalWidth(w); ok {
		return width
	}
	return defaultWidth
}

type terminalWidthAware interface {
	terminalWidth() (int, bool)
}

func terminalWidth(w io.Writer) (int, bool) {
	if aware, ok := w.(terminalWidthAware); ok {
		return aware.terminalWidth()
	}

	file, ok := w.(*os.File)
	if !ok {
		return 0, false
	}

	width, _, err := term.GetSize(int(file.Fd()))
	if err == nil && width > 0 {
		return width, true
	}

	if !isTerminalFile(file) {
		return 0, false
	}

	if width, ok := columnsEnvWidth(); ok {
		return width, true
	}

	return 0, false
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}

	switch file {
	case os.Stdout:
		return stdoutIsTerminal()
	case os.Stderr:
		return term.IsTerminal(int(file.Fd()))
	default:
		return term.IsTerminal(int(file.Fd()))
	}
}

func columnsEnvWidth() (int, bool) {
	value := strings.TrimSpace(os.Getenv("COLUMNS"))
	if value == "" {
		return 0, false
	}

	width, err := strconv.Atoi(value)
	if err != nil || width <= 0 {
		return 0, false
	}
	return width, true
}

func validateImageURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "https", "http":
		return nil
	default:
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}

const maxRedirects = 3

var imageClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("too many redirects")
		}
		if err := validateImageURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	},
}

func fetchAndDecode(ctx context.Context, imageURL string) (image.Image, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateImageURL(imageURL); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBodySize {
		return nil, fmt.Errorf("body too large")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if cfg.Width > maxDimension || cfg.Height > maxDimension {
		return nil, fmt.Errorf("image too large: %dx%d", cfg.Width, cfg.Height)
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func renderHalfBlock(w io.Writer, img image.Image, maxWidth int) {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return
	}

	// Scale to fit maxWidth, preserving aspect ratio.
	// Each terminal cell is roughly twice as tall as wide, so one char = 2 vertical pixels.
	// Cap height at maxWidth pixels (maxWidth/2 terminal rows) to prevent tall images
	// from producing excessive output.
	dstW := maxWidth
	dstH := (srcH * dstW) / srcW
	if dstH == 0 {
		dstH = 1
	}
	if dstH > maxWidth {
		dstW = (srcW * maxWidth) / srcH
		dstH = maxWidth
		if dstW == 0 {
			dstW = 1
		}
	}
	// Round height up to even for half-block pairing
	if dstH%2 != 0 {
		dstH++
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	// ~40 bytes per pixel cell + reset per row
	var sb strings.Builder
	sb.Grow(dstW * (dstH / 2) * 44)
	for y := 0; y < dstH; y += 2 {
		for x := 0; x < dstW; x++ {
			top := dst.RGBAAt(x, y)
			bot := dst.RGBAAt(x, y+1)
			sb.WriteString("\033[48;2;")
			sb.WriteString(strconv.FormatUint(uint64(top.R), 10))
			sb.WriteByte(';')
			sb.WriteString(strconv.FormatUint(uint64(top.G), 10))
			sb.WriteByte(';')
			sb.WriteString(strconv.FormatUint(uint64(top.B), 10))
			sb.WriteString("m\033[38;2;")
			sb.WriteString(strconv.FormatUint(uint64(bot.R), 10))
			sb.WriteByte(';')
			sb.WriteString(strconv.FormatUint(uint64(bot.G), 10))
			sb.WriteByte(';')
			sb.WriteString(strconv.FormatUint(uint64(bot.B), 10))
			sb.WriteString("m▄")
		}
		sb.WriteString("\033[0m\n")
	}
	fmt.Fprint(w, sb.String())
}
