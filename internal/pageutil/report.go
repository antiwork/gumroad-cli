package pageutil

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

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

type RenderResult struct {
	Action       string
	Source       string
	BeforeHTML   string
	AfterHTML    string
	LandingURL   string
	Report       SanitizationReport
	ClearMessage string
}

func RenderSanitizationResult(opts cmdutil.Options, result RenderResult) error {
	if opts.PlainOutput {
		return renderPlainReport(opts, result)
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	title := result.Action
	if result.Source != "" {
		title += " " + result.Source
	}
	if err := output.Writeln(opts.Out(), style.Bold(title)); err != nil {
		return err
	}

	before := utf8.RuneCountInString(result.BeforeHTML)
	after := utf8.RuneCountInString(result.AfterHTML)
	if err := output.Writef(opts.Out(), "HTML: %d -> %d chars (%s)\n", before, after, deltaLabel(after-before)); err != nil {
		return err
	}

	if err := renderHumanReport(opts, result.Report); err != nil {
		return err
	}
	if result.LandingURL != "" {
		if err := output.Writef(opts.Out(), "Live at %s\n", result.LandingURL); err != nil {
			return err
		}
	}
	if result.ClearMessage != "" {
		if err := output.Writeln(opts.Out(), result.ClearMessage); err != nil {
			return err
		}
	}
	return nil
}

func renderPlainReport(opts cmdutil.Options, result RenderResult) error {
	before := utf8.RuneCountInString(result.BeforeHTML)
	after := utf8.RuneCountInString(result.AfterHTML)
	rows := [][]string{{
		"summary",
		result.Action,
		result.Source,
		strconv.Itoa(before),
		strconv.Itoa(after),
		strconv.Itoa(after - before),
		strconv.Itoa(result.Report.TotalRemoved),
		strconv.FormatBool(result.Report.Truncated),
	}}

	for _, removed := range result.Report.RemovedTags {
		rows = append(rows, []string{
			"removed_tag",
			cleanReportValue(removed.Tag),
			formatAttrs(removed.Attrs),
			cleanReportValue(removed.Reason),
		})
	}
	for _, removed := range result.Report.RemovedAttributes {
		rows = append(rows, []string{
			"removed_attribute",
			cleanReportValue(removed.Tag),
			cleanReportValue(removed.Attribute),
			cleanReportValue(removed.Value),
			cleanReportValue(removed.Reason),
		})
	}
	if result.LandingURL != "" {
		rows = append(rows, []string{"landing_url", result.LandingURL})
	}
	if result.ClearMessage != "" {
		rows = append(rows, []string{"message", result.ClearMessage})
	}

	return output.PrintPlain(opts.Out(), rows)
}

func renderHumanReport(opts cmdutil.Options, report SanitizationReport) error {
	if report.TotalRemoved == 0 {
		return output.Writeln(opts.Out(), "No sanitization changes.")
	}

	suffix := ""
	if report.Truncated {
		suffix = " (showing first 100)"
	}
	if err := output.Writef(opts.Out(), "Sanitization removed %d item%s%s.\n", report.TotalRemoved, plural(report.TotalRemoved), suffix); err != nil {
		return err
	}

	if len(report.RemovedTags) > 0 {
		tbl := output.NewStyledTable(opts.Style(), "TAG", "ATTRS", "REASON")
		for _, removed := range report.RemovedTags {
			tbl.AddRow(
				truncateReportValue(cleanReportValue(removed.Tag)),
				truncateReportValue(formatAttrs(removed.Attrs)),
				truncateReportValue(cleanReportValue(removed.Reason)),
			)
		}
		if err := tbl.Render(opts.Out()); err != nil {
			return err
		}
	}

	if len(report.RemovedAttributes) > 0 {
		tbl := output.NewStyledTable(opts.Style(), "TAG", "ATTRIBUTE", "VALUE", "REASON")
		for _, removed := range report.RemovedAttributes {
			tbl.AddRow(
				truncateReportValue(cleanReportValue(removed.Tag)),
				truncateReportValue(cleanReportValue(removed.Attribute)),
				truncateReportValue(cleanReportValue(removed.Value)),
				truncateReportValue(cleanReportValue(removed.Reason)),
			)
		}
		if err := tbl.Render(opts.Out()); err != nil {
			return err
		}
	}

	return nil
}

func deltaLabel(delta int) string {
	switch {
	case delta > 0:
		return fmt.Sprintf("+%d", delta)
	case delta < 0:
		return strconv.Itoa(delta)
	default:
		return "no change"
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func formatAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}

	cleaned := make(map[string]string, len(attrs))
	for k, v := range attrs {
		cleaned[cleanReportValue(k)] = cleanReportValue(v)
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return ""
	}
	return string(data)
}

func cleanReportValue(value string) string {
	return strings.TrimSpace(StripTerminalControls(value))
}

func truncateReportValue(value string) string {
	const maxRunes = 120
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}

	var b strings.Builder
	count := 0
	for _, r := range value {
		if count >= maxRunes-3 {
			break
		}
		b.WriteRune(r)
		count++
	}
	b.WriteString("...")
	return b.String()
}
