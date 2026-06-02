package pageutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
)

func TestRenderSanitizationResultStripsReportControls(t *testing.T) {
	var out bytes.Buffer
	opts := cmdutil.DefaultOptions()
	opts.Stdout = &out
	opts.NoColor = true

	err := RenderSanitizationResult(opts, RenderResult{
		Action:     "Previewed page",
		Source:     "landing.html",
		BeforeHTML: "<a>Buy</a>",
		AfterHTML:  "<a>Buy</a>",
		Report: SanitizationReport{
			RemovedAttributes: []RemovedAttribute{{
				Tag:       "a",
				Attribute: "href",
				Value:     "\x1b[31mjavascript:alert(1)\x1b[0m\n",
				Reason:    "javascript: URL blocked\x00",
			}},
			TotalRemoved: 1,
		},
	})
	if err != nil {
		t.Fatalf("RenderSanitizationResult returned error: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\x00") {
		t.Fatalf("expected terminal controls to be stripped, got %q", got)
	}
	if !strings.Contains(got, "javascript:alert(1)") || !strings.Contains(got, "javascript: URL blocked") {
		t.Fatalf("expected sanitized report values, got %q", got)
	}
}

func TestRenderSanitizationResultPlainRows(t *testing.T) {
	var out bytes.Buffer
	opts := cmdutil.DefaultOptions()
	opts.Stdout = &out
	opts.PlainOutput = true

	err := RenderSanitizationResult(opts, RenderResult{
		Action:     "Published page",
		Source:     "landing.html",
		BeforeHTML: "<script></script>",
		AfterHTML:  "",
		LandingURL: "https://seller.gumroad.com/l/prod",
		Report: SanitizationReport{
			RemovedTags: []RemovedTag{{
				Tag:    "script",
				Attrs:  map[string]string{"src": "\x1b[31mhttps://evil.test/x.js\x1b[0m"},
				Reason: "script src host not allowed",
			}},
			TotalRemoved: 1,
		},
	})
	if err != nil {
		t.Fatalf("RenderSanitizationResult returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "summary\tPublished page\tlanding.html") {
		t.Fatalf("plain output missing summary row: %q", got)
	}
	if !strings.Contains(got, "removed_tag\tscript") {
		t.Fatalf("plain output missing removed tag row: %q", got)
	}
	if !strings.Contains(got, "landing_url\thttps://seller.gumroad.com/l/prod") {
		t.Fatalf("plain output missing landing URL row: %q", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected terminal controls to be stripped, got %q", got)
	}
}
