package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type readinessCategory struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Weight   int      `json:"weight"`
	Score    int      `json:"score"`
	Severity string   `json:"severity"`
	Note     string   `json:"note"`
	Details  []string `json:"details"`
}

type readinessPayload struct {
	Overall    int                 `json:"overall"`
	Severity   string              `json:"severity"`
	Categories []readinessCategory `json:"categories"`
	ComputedAt string              `json:"computed_at"`
}

type readinessResponse struct {
	Readiness readinessPayload `json:"readiness"`
}

func newReadinessCmd() *cobra.Command {
	var showDetails bool
	cmd := &cobra.Command{
		Use:   "readiness <id>",
		Short: "Show the readiness score for a product",
		Long: "Show the deterministic readiness score for a product page.\n\n" +
			"The score is computed server-side from five weighted dimensions " +
			"(name, description, cover, pricing, social proof) and returned with " +
			"per-category notes. Pipe through --json or --jq to apply your own " +
			"AI-graded sub-rules in your agent of choice.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad products readiness <id>
  gumroad products readiness <id> --details
  gumroad products readiness <id> --json
  gumroad products readiness <id> --jq '.readiness.overall'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath("products", args[0], "readiness")
			return cmdutil.RunRequest(opts, "Fetching readiness score...", "GET", path, url.Values{}, func(data json.RawMessage) error {
				var resp readinessResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}
				return renderReadiness(opts, resp.Readiness, showDetails)
			})
		},
	}
	cmd.Flags().BoolVar(&showDetails, "details", false, "Show per-category details (bullet list explaining each score)")
	return cmd
}

func renderReadiness(opts cmdutil.Options, r readinessPayload, showDetails bool) error {
	if opts.PlainOutput {
		rows := make([][]string, 0, len(r.Categories)+1)
		rows = append(rows, []string{"overall", fmt.Sprintf("%d", r.Overall), r.Severity, ""})
		for _, cat := range r.Categories {
			rows = append(rows, []string{cat.Key, fmt.Sprintf("%d", cat.Score), cat.Severity, cat.Note})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	style := opts.Style()
	overallColored := colorBySeverity(style, r.Severity, fmt.Sprintf("%d/100", r.Overall))
	if err := output.Writeln(opts.Out(), style.Bold("Page readiness")); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Overall: %s  (%s)\n\n", overallColored, severityLabel(r.Severity)); err != nil {
		return err
	}

	tbl := output.NewTable("CATEGORY", "WEIGHT", "SCORE", "SEVERITY", "NOTE")
	for _, cat := range r.Categories {
		tbl.AddRow(
			cat.Label,
			fmt.Sprintf("%d%%", cat.Weight),
			fmt.Sprintf("%d", cat.Score),
			colorBySeverity(style, cat.Severity, cat.Severity),
			cat.Note,
		)
	}
	if err := output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		return tbl.Render(w)
	}); err != nil {
		return err
	}

	if showDetails {
		if err := output.Writeln(opts.Out(), ""); err != nil {
			return err
		}
		for _, cat := range r.Categories {
			if err := output.Writeln(opts.Out(), style.Bold(cat.Label)); err != nil {
				return err
			}
			for _, d := range cat.Details {
				if err := output.Writef(opts.Out(), "  - %s\n", d); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func colorBySeverity(style output.Styler, severity, text string) string {
	switch severity {
	case "good":
		return style.Green(text)
	case "ok":
		return style.Yellow(text)
	case "weak":
		return style.Yellow(text)
	case "missing":
		return style.Red(text)
	default:
		return text
	}
}

func severityLabel(severity string) string {
	switch severity {
	case "good":
		return "Strong"
	case "ok":
		return "Decent"
	case "weak":
		return "Needs work"
	case "missing":
		return "Weak"
	default:
		return severity
	}
}
