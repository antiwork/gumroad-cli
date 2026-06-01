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
	srv := httptest.NewServer(devHandler(state, "landing.html", "https://creator.example/l/prod?wanted=true", devFieldValues{}))
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
		`e.origin !== "null"`,
		`var checkoutURL = "https://creator.example/l/prod?wanted=true"`,
		`allowedCheckoutKeys = ["variant", "option", "quantity", "price", "recurrence"]`,
		`e.data.type === "gumroad:checkout"`,
		`window.location.href = buildCheckoutURL(e.data.params)`,
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
	state := &devState{html: `<a data-gumroad-action="buy" data-gumroad-option="Pro" data-gumroad-recurrence="yearly">Buy</a><button data-gumroad-action="buy" data-gumroad-quantity="2">Buy</button>`, clients: map[chan struct{}]struct{}{}}
	srv := httptest.NewServer(devHandler(state, "landing.html", "https://creator.example/l/prod?wanted=true", devFieldValues{}))
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
		`["data-gumroad-option", "variant"]`,
		`["data-gumroad-quantity", "quantity"]`,
		`["data-gumroad-price", "price"]`,
		`["data-gumroad-recurrence", "recurrence"]`,
		`document.querySelectorAll('[data-gumroad-action="buy"]')`,
		`el.setAttribute("href", buildCheckoutURL(params))`,
		`parent.postMessage({ type: "gumroad:checkout", params: params }, "*")`,
		`return false`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("embed missing %q in %s", want, body)
		}
	}
}

func TestDevEmbedInterpolatesProductFields(t *testing.T) {
	state := &devState{html: `<h1 data-gumroad-field="name">Fallback</h1><span data-gumroad-field="price">$0</span><p data-gumroad-field="description">Fallback description</p>`, clients: map[chan struct{}]struct{}{}}
	srv := httptest.NewServer(devHandler(state, "landing.html", "https://creator.example/l/prod?wanted=true", devFieldValues{
		Name:        "Live Product",
		Price:       "$9",
		Description: "<p>Live <strong>description</strong></p>",
	}))
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
		`var gumroadFieldValues = {"name":"Live Product","price":"$9","description":"\u003cp\u003eLive \u003cstrong\u003edescription\u003c/strong\u003e\u003c/p\u003e"}`,
		`document.querySelectorAll("[data-gumroad-field]")`,
		`el.textContent = value`,
		`textFromHTML(gumroadFieldValues.description)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("embed missing %q in %s", want, body)
		}
	}
}
