package pageutil

import (
	"strings"
	"testing"
)

func TestReportPlainRowsStripsUserControlledValues(t *testing.T) {
	report := SanitizationReport{
		RemovedTags: []RemovedTag{{
			Tag:    "script\x1b[31m",
			Attrs:  map[string]string{"src": "https://evil.example/x\x1b[0m.js"},
			Reason: "script src host not allowed\n",
		}},
		RemovedAttributes: []RemovedAttribute{{
			Tag:       "a",
			Attribute: "href",
			Value:     "javascript:alert(1)\x00",
			Reason:    "javascript: URL blocked",
		}},
		TotalRemoved: 2,
	}

	rows := ReportPlainRows("<script></script>", nil, report)
	for _, row := range rows {
		for _, value := range row {
			if value == "script\x1b[31m" || value == "script src host not allowed\n" || value == "javascript:alert(1)\x00" {
				t.Fatalf("unsafe value leaked in rows: %#v", rows)
			}
		}
	}
}

func TestRenderUnifiedDiffShowsChangedLines(t *testing.T) {
	from := "<h1>Old</h1>\n<p>Keep</p>\n"
	to := "<h1>New</h1>\n<p>Keep</p>\n"
	var out strings.Builder

	if err := RenderUnifiedDiff(&out, "current", &from, "preview", &to); err != nil {
		t.Fatalf("RenderUnifiedDiff failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "--- current") || !strings.Contains(got, "+++ preview") {
		t.Fatalf("missing diff headers: %q", got)
	}
	if !strings.Contains(got, "-<h1>Old</h1>") || !strings.Contains(got, "+<h1>New</h1>") {
		t.Fatalf("missing changed lines: %q", got)
	}
}

func TestRenderUnifiedDiffSkipsLargeDiff(t *testing.T) {
	from := strings.Repeat("a\n", maxDiffCells/2)
	to := "b\nb\n"
	var out strings.Builder

	if err := RenderUnifiedDiff(&out, "current", &from, "preview", &to); err != nil {
		t.Fatalf("RenderUnifiedDiff failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Diff skipped") {
		t.Fatalf("expected skipped diff, got %q", got)
	}
	if strings.Contains(got, "@@") {
		t.Fatalf("expected no generated diff body, got %q", got)
	}
}
