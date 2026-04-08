package output

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func solidImage(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func servePNG(t *testing.T, img image.Image) *httptest.Server {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
}

func TestRenderHalfBlock_BasicOutput(t *testing.T) {
	setColorEnabledForTest(t, true)

	img := solidImage(4, 4, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	renderHalfBlock(&buf, img, 4)

	out := buf.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines for 4px tall image, got %d", len(lines))
	}
	if !strings.Contains(out, "▄") {
		t.Error("expected half-block characters in output")
	}
	if !strings.Contains(out, "\033[48;2;") {
		t.Error("expected ANSI background color codes")
	}
	if !strings.Contains(out, "\033[38;2;") {
		t.Error("expected ANSI foreground color codes")
	}
	for _, line := range lines {
		if !strings.HasSuffix(line, "\033[0m") {
			t.Errorf("line should end with reset: %q", line)
		}
	}
}

func TestRenderHalfBlock_OddHeight(t *testing.T) {
	setColorEnabledForTest(t, true)

	img := solidImage(4, 5, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	renderHalfBlock(&buf, img, 4)

	out := buf.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	// 5px tall → scales to fit 4 wide, height rounds up to even → 3 lines
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines for 5px tall image, got %d", len(lines))
	}
	// Main check: no panic
}

func TestRenderHalfBlock_ScalesDown(t *testing.T) {
	setColorEnabledForTest(t, true)

	img := solidImage(200, 200, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	renderHalfBlock(&buf, img, 10)

	out := buf.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > 10 {
		t.Errorf("expected at most 10 lines for maxWidth=10, got %d", len(lines))
	}
}

func TestRenderHalfBlock_TallImage(t *testing.T) {
	setColorEnabledForTest(t, true)

	img := solidImage(4, 400, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	renderHalfBlock(&buf, img, 40)

	out := buf.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	// maxWidth=40, so height capped at 40px → 20 terminal lines
	if len(lines) > 20 {
		t.Errorf("expected at most 20 lines for tall image, got %d", len(lines))
	}
}

func TestRenderImage_EmptyURL(t *testing.T) {
	setColorEnabledForTest(t, true)

	var buf bytes.Buffer
	RenderImage(&buf, "", 40)
	if buf.Len() != 0 {
		t.Error("expected no output for empty URL")
	}
}

func TestRenderImage_ColorDisabled(t *testing.T) {
	setColorEnabledForTest(t, false)

	var buf bytes.Buffer
	RenderImage(&buf, "http://example.com/img.png", 40)
	if buf.Len() != 0 {
		t.Error("expected no output when color is disabled")
	}
}

func TestRenderImage_HTTPSuccess(t *testing.T) {
	setColorEnabledForTest(t, true)

	img := solidImage(8, 8, color.RGBA{R: 128, G: 64, B: 32, A: 255})
	srv := servePNG(t, img)
	defer srv.Close()

	var buf bytes.Buffer
	RenderImage(&buf, srv.URL+"/test.png", 8)
	out := buf.String()
	if !strings.Contains(out, "▄") {
		t.Error("expected half-block characters in output")
	}
}

func TestRenderImage_WebPSuccess(t *testing.T) {
	setColorEnabledForTest(t, true)

	webpData, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	if err != nil {
		t.Fatalf("decode webp fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webpData)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	RenderImage(&buf, srv.URL+"/test.webp", 8)
	out := buf.String()
	if !strings.Contains(out, "▄") {
		t.Error("expected half-block characters in output for webp")
	}
}

func TestRenderImage_HTTP404(t *testing.T) {
	setColorEnabledForTest(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	RenderImage(&buf, srv.URL+"/missing.png", 40)
	if buf.Len() != 0 {
		t.Error("expected no output for 404")
	}
}

func TestRenderImage_InvalidBody(t *testing.T) {
	setColorEnabledForTest(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not an image"))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	RenderImage(&buf, srv.URL+"/bad", 40)
	if buf.Len() != 0 {
		t.Error("expected no output for invalid image data")
	}
}

func TestRenderImageWithContext_Canceled(t *testing.T) {
	setColorEnabledForTest(t, true)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not reached"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	RenderImageWithContext(ctx, &buf, srv.URL+"/test.png", 8)
	if hits.Load() != 0 {
		t.Fatalf("expected canceled context to skip HTTP fetch, got %d requests", hits.Load())
	}
	if buf.Len() != 0 {
		t.Fatal("expected no output for canceled context")
	}
}

func TestRenderImage_SlowServerTimesOutQuickly(t *testing.T) {
	setColorEnabledForTest(t, true)

	var hits atomic.Int32
	slowResponseDelay := fetchTimeout + 250*time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(slowResponseDelay)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("too late"))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	start := time.Now()
	RenderImage(&buf, srv.URL+"/slow.png", 8)
	elapsed := time.Since(start)

	if hits.Load() != 1 {
		t.Fatalf("expected one fetch attempt, got %d", hits.Load())
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output for timed out image fetch, got %q", buf.String())
	}
	if elapsed >= 2*fetchTimeout {
		t.Fatalf("image render took too long: %s", elapsed)
	}
}

func TestRenderImage_OversizedDimensions(t *testing.T) {
	setColorEnabledForTest(t, true)

	// Craft a minimal PNG with oversized IHDR dimensions
	var buf bytes.Buffer
	// PNG signature
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	// IHDR chunk: length=13, type=IHDR, data, CRC
	ihdrData := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdrData[0:4], 5000) // width
	binary.BigEndian.PutUint32(ihdrData[4:8], 5000) // height
	ihdrData[8] = 8                                 // bit depth
	ihdrData[9] = 2                                 // color type (truecolor)
	ihdrData[10] = 0                                // compression
	ihdrData[11] = 0                                // filter
	ihdrData[12] = 0                                // interlace

	var chunk bytes.Buffer
	lenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBytes, 13)
	chunk.Write(lenBytes)
	chunk.Write([]byte("IHDR"))
	chunk.Write(ihdrData)
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdrData)
	crcBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(crcBytes, crc.Sum32())
	chunk.Write(crcBytes)
	buf.Write(chunk.Bytes())

	pngData := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer srv.Close()

	var outBuf bytes.Buffer
	RenderImage(&outBuf, srv.URL+"/huge.png", 40)
	if outBuf.Len() != 0 {
		t.Error("expected no output for oversized image")
	}
}

func TestFetchAndDecode_RejectsFileScheme(t *testing.T) {
	_, err := fetchAndDecode(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error for file:// scheme")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchAndDecode_RejectsFTPScheme(t *testing.T) {
	_, err := fetchAndDecode(context.Background(), "ftp://example.com/image.png")
	if err == nil {
		t.Fatal("expected error for ftp:// scheme")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("expected validateImageURL rejection, got: %v", err)
	}
}

func TestFetchAndDecode_LimitsRedirects(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	defer srv.Close()

	_, err := fetchAndDecode(context.Background(), srv.URL+"/loop.png")
	if err == nil {
		t.Fatal("expected error for redirect loop")
	}
	// 1 initial + (maxRedirects-1) followed redirects = maxRedirects total hits
	// (the last redirect is rejected before being sent)
	if got := hits.Load(); got != int32(maxRedirects) {
		t.Fatalf("expected %d requests, got %d", maxRedirects, got)
	}
}

func TestImageEnabled_FollowsColor(t *testing.T) {
	setColorEnabledForTest(t, true)
	if !ImageEnabled() {
		t.Error("ImageEnabled should be true when color is enabled")
	}
	setColorEnabledForTest(t, false)
	if ImageEnabled() {
		t.Error("ImageEnabled should be false when color is disabled")
	}
}

func TestTerminalWidth_Fallback(t *testing.T) {
	t.Setenv("COLUMNS", "")

	w := TerminalWidth(80)
	if w != 80 {
		t.Errorf("got %d, want 80", w)
	}
}

func TestTerminalWidthFor_NonTerminalWriterIgnoresColumnsEnv(t *testing.T) {
	t.Setenv("COLUMNS", "91")

	if got := TerminalWidthFor(&bytes.Buffer{}, 77); got != 77 {
		t.Fatalf("got %d, want 77", got)
	}
}

func TestTerminalWidthFor_IgnoresInvalidColumnsEnv(t *testing.T) {
	t.Setenv("COLUMNS", "nope")

	if got := TerminalWidthFor(&bytes.Buffer{}, 77); got != 77 {
		t.Fatalf("got %d, want 77", got)
	}
}
