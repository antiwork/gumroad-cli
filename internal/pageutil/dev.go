package pageutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/cli/browser"
)

const (
	DefaultDevPort = 4747
	devCSP         = "default-src 'none'; script-src 'unsafe-inline' https://cdn.tailwindcss.com https://cdn.jsdelivr.net https://unpkg.com; style-src 'unsafe-inline' https://cdn.tailwindcss.com https://fonts.googleapis.com https://fonts.bunny.net; img-src data: blob: https:; font-src data: https://fonts.gstatic.com https://fonts.bunny.net; connect-src 'none'; form-action 'self';"
	devDebounce    = 450 * time.Millisecond
)

type devState struct {
	mu      sync.RWMutex
	html    string
	report  SanitizationReport
	clients map[chan struct{}]struct{}
}

func Dev(opts cmdutil.Options, target Target, path string, port int, shouldOpen bool) error {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath
	}
	if path == "-" {
		return fmt.Errorf("page dev needs a file path; use page preview or page push for stdin")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}

	_, source, err := ReadHTML(opts, path)
	if err != nil {
		return err
	}

	token, err := config.Token()
	if err != nil {
		return err
	}
	client := cmdutil.NewAPIClient(opts, token)

	showData, err := client.Get(target.Path, url.Values{})
	if err != nil {
		return err
	}
	show, err := cmdutil.DecodeJSON[ShowResponse](showData)
	if err != nil {
		return err
	}
	if show.Product.LandingURL == "" {
		return fmt.Errorf("product response did not include landing_url")
	}

	initialHTML, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", path, err)
	}
	preview, err := previewForDev(client, target, string(initialHTML))
	if err != nil {
		return err
	}

	state := &devState{html: stringOrEmpty(preview.CustomHTML), report: preview.SanitizationReport, clients: map[chan struct{}]struct{}{}}
	checkoutURL := wantedURL(show.Product.LandingURL)
	handler := devHandler(state, source, checkoutURL)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("could not start dev server: %w", err)
	}
	localURL := "http://" + listener.Addr().String()

	ctx, cancel := context.WithCancel(opts.Context)
	defer cancel()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan error, 1)
	shutdownDone := make(chan struct{})
	defer close(shutdownDone)
	go func() {
		err := server.Serve(listener)
		if errorsIsServerClosed(err) {
			err = nil
		}
		done <- err
	}()
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			cancel()
		}
		select {
		case <-signals:
			_ = server.Close()
		case <-shutdownDone:
		}
	}()
	go watchDevFile(ctx, opts, client, target, path, state)

	if err := RenderReport(opts, source, string(initialHTML), preview.CustomHTML, preview.SanitizationReport); err != nil {
		return err
	}
	if !opts.Quiet {
		if err := output.Writeln(opts.Out(), "Dev server: "+localURL); err != nil {
			return err
		}
		if err := output.Writeln(opts.Out(), "Checkout bridge: "+checkoutURL); err != nil {
			return err
		}
	}
	if shouldOpen {
		if err := browser.OpenURL(localURL); err != nil {
			return fmt.Errorf("could not open browser: %w", err)
		}
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return <-done
	case err := <-done:
		return err
	}
}

func previewForDev(client *api.Client, target Target, html string) (PreviewResponse, error) {
	data, err := client.PostJSON(target.PreviewPath, pageHTMLPayload{CustomHTML: html})
	if err != nil {
		return PreviewResponse{}, withRateLimitHint(err, "60 previews/min per token", "Pause briefly before previewing again.")
	}
	return cmdutil.DecodeJSON[PreviewResponse](data)
}

func watchDevFile(ctx context.Context, opts cmdutil.Options, client *api.Client, target Target, path string, state *devState) {
	last := fileSignature(path)
	var pending bool
	var due time.Time
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			current := fileSignature(path)
			if current != last {
				last = current
				pending = true
				due = now.Add(devDebounce)
				continue
			}
			if !pending || now.Before(due) {
				continue
			}
			pending = false
			reloadDevPreview(opts, client, target, path, state)
		}
	}
}

func reloadDevPreview(opts cmdutil.Options, client *api.Client, target Target, path string, state *devState) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !opts.Quiet {
			_, _ = fmt.Fprintf(opts.Err(), "Could not read %s: %v\n", path, err)
		}
		return
	}
	preview, err := previewForDev(client, target, string(data))
	if err != nil {
		if !opts.Quiet {
			_, _ = fmt.Fprintf(opts.Err(), "Preview failed: %v\n", err)
		}
		return
	}

	state.mu.Lock()
	state.html = stringOrEmpty(preview.CustomHTML)
	state.report = preview.SanitizationReport
	clients := make([]chan struct{}, 0, len(state.clients))
	for client := range state.clients {
		clients = append(clients, client)
	}
	state.mu.Unlock()

	if err := RenderReport(opts, path, string(data), preview.CustomHTML, preview.SanitizationReport); err != nil && !opts.Quiet {
		_, _ = fmt.Fprintf(opts.Err(), "Could not render report: %v\n", err)
	}
	for _, client := range clients {
		select {
		case client <- struct{}{}:
		default:
		}
	}
}

func devHandler(state *devState, title string, checkoutURL string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, devWrapperDocument(title, checkoutURL))
	})
	mux.HandleFunc("/embed", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		current := state.html
		state.mu.RUnlock()
		w.Header().Set("Content-Security-Policy", devCSP)
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, customHTMLDocument(current, checkoutURL))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch := make(chan struct{}, 1)
		state.mu.Lock()
		state.clients[ch] = struct{}{}
		state.mu.Unlock()
		defer func() {
			state.mu.Lock()
			delete(state.clients, ch)
			state.mu.Unlock()
		}()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				_, _ = fmt.Fprint(w, "event: reload\ndata: now\n\n")
				flusher.Flush()
			}
		}
	})
	return mux
}

func customHTMLDocument(customHTML string, checkoutURL string) string {
	checkout, _ := json.Marshal(checkoutURL)
	return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <script src="https://cdn.tailwindcss.com"></script>
  </head>
  <body>
` + customHTML + `
    <script>
      (function () {
        var checkoutURL = ` + string(checkout) + `;
        var checkoutParamAttributes = [
          ["data-gumroad-option", "variant"],
          ["data-gumroad-quantity", "quantity"],
          ["data-gumroad-price", "price"],
          ["data-gumroad-recurrence", "recurrence"]
        ];
        var checkoutKeys = ["variant", "option", "quantity", "price", "recurrence"];
        function collectCheckoutParams(el) {
          var params = {};
          checkoutParamAttributes.forEach(function (pair) {
            var value = el.getAttribute(pair[0]);
            if (value !== null && value !== "") {
              params[pair[1]] = value;
            }
          });
          return params;
        }
        function buildCheckoutURL(params) {
          var url = new URL(checkoutURL, window.location.href);
          checkoutKeys.forEach(function (key) {
            var value = params[key];
            if (typeof value === "string" && value !== "") {
              url.searchParams.set(key, value);
            }
          });
          return url.toString();
        }
        document.querySelectorAll('[data-gumroad-action="buy"]').forEach(function (el) {
          var params = collectCheckoutParams(el);
          if (el.tagName && el.tagName.toLowerCase() === "a") {
            el.setAttribute("href", buildCheckoutURL(params));
          }
          el.onclick = function (event) {
            if (event) event.preventDefault();
            parent.postMessage({ type: "gumroad:checkout", params: params }, "*");
            return false;
          };
        });
      })();
    </script>
  </body>
</html>
`
}

func devWrapperDocument(title string, checkoutURL string) string {
	title = html.EscapeString(filepath.Base(title))
	checkout, _ := json.Marshal(checkoutURL)
	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>` + title + `</title>
    <style>html,body{margin:0;padding:0;height:100%;overflow:hidden}iframe{display:block;width:100%;height:100%;border:0}</style>
  </head>
  <body>
    <iframe id="gumroad-landing-frame" src="/embed" title="` + title + `" sandbox="allow-scripts allow-forms"></iframe>
    <script>
      var frame = document.getElementById("gumroad-landing-frame");
      var checkoutURL = ` + string(checkout) + `;
      var allowedCheckoutKeys = ["variant", "option", "quantity", "price", "recurrence"];
      function buildCheckoutURL(params) {
        var url = new URL(checkoutURL, window.location.href);
        if (!params || typeof params !== "object" || Array.isArray(params)) {
          return url.toString();
        }
        allowedCheckoutKeys.forEach(function (key) {
          var value = params[key];
          if (typeof value === "string" && value !== "") {
            url.searchParams.set(key, value);
          }
        });
        return url.toString();
      }
      window.addEventListener("message", function (e) {
        if (e.source !== frame.contentWindow || e.origin !== "null") {
          return;
        }
        if (e.data === "gumroad:checkout") {
          window.location.href = checkoutURL;
          return;
        }
        if (e.data && e.data.type === "gumroad:checkout") {
          window.location.href = buildCheckoutURL(e.data.params);
        }
      });
      var events = new EventSource("/events");
      events.addEventListener("reload", function () {
        frame.src = "/embed?reload=" + Date.now();
      });
    </script>
  </body>
</html>
`
}

func wantedURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	values := parsed.Query()
	values.Set("wanted", "true")
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func fileSignature(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func errorsIsServerClosed(err error) bool {
	return err == nil || errors.Is(err, http.ErrServerClosed)
}
