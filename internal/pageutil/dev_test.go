package pageutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevHandlerServesEmbedWithProductionHeaders(t *testing.T) {
	state := &devState{html: "<main>Dev</main>", clients: map[chan struct{}]struct{}{}}
	srv := httptest.NewServer(devHandler(state, "landing.html", "https://creator.example/l/prod?wanted=true"))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/embed")
	if err != nil {
		t.Fatalf("GET /embed failed: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Security-Policy"); got != devCSP {
		t.Fatalf("CSP = %q", got)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if got := resp.Header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
}

func TestDevWrapperContainsCheckoutBridgeAndReloadStream(t *testing.T) {
	doc := devWrapperDocument("landing.html", "https://creator.example/l/prod?wanted=true")
	for _, want := range []string{
		`e.data === "gumroad:checkout"`,
		`e.origin === "null"`,
		`window.location.href = "https://creator.example/l/prod?wanted=true"`,
		`new EventSource("/events")`,
		`frame.src = "/embed?reload=" + Date.now()`,
		`sandbox="allow-scripts allow-forms"`,
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("wrapper missing %q in %s", want, doc)
		}
	}
}

func TestDevEmbedWiresBuyActionElements(t *testing.T) {
	state := &devState{html: `<a data-gumroad-action="buy" href="#">Buy</a><button data-gumroad-action="buy">Buy</button>`, clients: map[chan struct{}]struct{}{}}
	srv := httptest.NewServer(devHandler(state, "landing.html", "https://creator.example/l/prod?wanted=true"))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/embed")
	if err != nil {
		t.Fatalf("GET /embed failed: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		`data-gumroad-action="buy"`,
		`var checkoutURL = "https://creator.example/l/prod?wanted=true"`,
		`document.querySelectorAll('[data-gumroad-action="buy"]')`,
		`el.setAttribute("href", checkoutURL)`,
		`parent.postMessage("gumroad:checkout", "*")`,
		`return false`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("embed missing %q in %s", want, body)
		}
	}
}
