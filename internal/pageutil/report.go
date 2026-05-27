package pageutil

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

const maxDiffCells = 4_000_000

type SanitizationReport struct {
	RemovedTags       []RemovedTag       `json:"removed_tags"`
	RemovedAttributes []RemovedAttribute `json:"removed_attributes"`
	TotalRemoved      int                `json:"total_removed"`
	Truncated         bool               `json:"truncated"`
}

type RemovedTag struct {
	Tag    string            `json:"tag"`
	Attrs  map[string]string `json:"attrs"`
	Reason string            `json:"reason"`
}

type RemovedAttribute struct {
	Tag       string `json:"tag"`
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Reason    string `json:"reason"`
}

func RenderReport(opts cmdutil.Options, source string, original string, sanitized *string, report SanitizationReport) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), ReportPlainRows(original, sanitized, report))
	}
	if opts.Quiet {
		return nil
	}
	return renderReportHuman(opts, source, original, sanitized, report)
}

func ReportPlainRows(original string, sanitized *string, report SanitizationReport) [][]string {
	originalBytes, sanitizedBytes, delta := reportSizes(original, sanitized)
	rows := [][]string{{
		"summary",
		strconv.Itoa(originalBytes),
		strconv.Itoa(sanitizedBytes),
		strconv.Itoa(delta),
		strconv.Itoa(report.TotalRemoved),
		strconv.FormatBool(report.Truncated),
	}}
	for _, item := range report.RemovedTags {
		rows = append(rows, []string{
			"removed_tag",
			cleanReportValue(item.Tag),
			"",
			formatAttrs(item.Attrs),
			cleanReportValue(item.Reason),
		})
	}
	for _, item := range report.RemovedAttributes {
		rows = append(rows, []string{
			"removed_attribute",
			cleanReportValue(item.Tag),
			cleanReportValue(item.Attribute),
			cleanReportValue(item.Value),
			cleanReportValue(item.Reason),
		})
	}
	return rows
}

func renderReportHuman(opts cmdutil.Options, source string, original string, sanitized *string, report SanitizationReport) error {
	style := opts.Style()
	originalBytes, sanitizedBytes, delta := reportSizes(original, sanitized)
	suffix := "no changes"
	if report.TotalRemoved == 1 {
		suffix = "1 item removed"
	} else if report.TotalRemoved > 1 {
		suffix = fmt.Sprintf("%d items removed", report.TotalRemoved)
	}
	if err := output.Writef(opts.Out(), "%s %s (%d bytes -> %d bytes, %+d): %s.\n", style.Bold("Sanitized"), source, originalBytes, sanitizedBytes, delta, suffix); err != nil {
		return err
	}
	if report.TotalRemoved == 0 {
		return nil
	}

	tbl := output.NewStyledTable(style, "TYPE", "TAG", "ATTRIBUTE", "VALUE", "REASON")
	for _, item := range report.RemovedTags {
		tbl.AddRow("tag", cleanReportValue(item.Tag), "", formatAttrs(item.Attrs), cleanReportValue(item.Reason))
	}
	for _, item := range report.RemovedAttributes {
		tbl.AddRow("attribute", cleanReportValue(item.Tag), cleanReportValue(item.Attribute), cleanReportValue(item.Value), cleanReportValue(item.Reason))
	}
	if err := tbl.Render(opts.Out()); err != nil {
		return err
	}
	if report.Truncated {
		return output.Writeln(opts.Out(), style.Dim("Report truncated; the server removed more entries than it returned."))
	}
	return nil
}

func reportSizes(original string, sanitized *string) (int, int, int) {
	sanitizedValue := ""
	if sanitized != nil {
		sanitizedValue = *sanitized
	}
	originalBytes := len([]byte(original))
	sanitizedBytes := len([]byte(sanitizedValue))
	return originalBytes, sanitizedBytes, sanitizedBytes - originalBytes
}

func cleanReportValue(value string) string {
	return StripTerminalControls(value)
}

func formatAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, cleanReportValue(key)+"="+cleanReportValue(attrs[key]))
	}
	return strings.Join(parts, " ")
}

func RenderUnifiedDiff(w io.Writer, fromName string, from *string, toName string, to *string) error {
	fromValue := ""
	if from != nil {
		fromValue = *from
	}
	toValue := ""
	if to != nil {
		toValue = *to
	}
	if fromValue == toValue {
		return output.Writeln(w, "No diff.")
	}

	if _, err := fmt.Fprintf(w, "--- %s\n+++ %s\n", fromName, toName); err != nil {
		return err
	}
	fromLines := splitDiffLines(fromValue)
	toLines := splitDiffLines(toValue)
	if diffTooLarge(fromLines, toLines) {
		return output.Writef(w, "Diff skipped: inputs have %d and %d lines; use --json to inspect custom_html.\n", len(fromLines), len(toLines))
	}

	for _, line := range simpleLineDiff(fromLines, toLines) {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func splitDiffLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.SplitAfter(value, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func diffTooLarge(a []string, b []string) bool {
	aCells := len(a) + 1
	bCells := len(b) + 1
	return aCells > maxDiffCells/bCells
}

func simpleLineDiff(a []string, b []string) []string {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	lines := []string{"@@"}
	for i, j := 0, 0; i < len(a) || j < len(b); {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			lines = append(lines, " "+strings.TrimSuffix(a[i], "\n"))
			i++
			j++
		case j < len(b) && (i == len(a) || dp[i][j+1] >= dp[i+1][j]):
			lines = append(lines, "+"+strings.TrimSuffix(b[j], "\n"))
			j++
		case i < len(a):
			lines = append(lines, "-"+strings.TrimSuffix(a[i], "\n"))
			i++
		}
	}
	return lines
}
