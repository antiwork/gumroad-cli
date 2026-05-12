package users

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

var relatedSignalOrder = []string{"ip", "payment_address", "card_fingerprint"}

var validRelatedSignals = map[string]bool{
	"ip":               true,
	"payment_address":  true,
	"card_fingerprint": true,
}

type relatedResponse struct {
	UserID           string          `json:"user_id"`
	SignalsEvaluated []string        `json:"signals_evaluated"`
	PerSignalLimit   api.JSONInt     `json:"per_signal_limit"`
	RelatedUsers     []relatedUser   `json:"related_users"`
	Truncated        map[string]bool `json:"truncated"`
}

type relatedUser struct {
	ID        string            `json:"id"`
	Email     string            `json:"email"`
	Name      string            `json:"name"`
	DeletedAt string            `json:"deleted_at"`
	RiskState riskState         `json:"risk_state"`
	Relations []relatedRelation `json:"relations"`
}

type relatedRelation struct {
	Signal      string   `json:"signal"`
	SharedValue string   `json:"shared_value"`
	Via         []string `json:"via"`
}

func newRelatedCmd() *cobra.Command {
	var (
		lookup  userLookupFlags
		signals []string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "related",
		Short: "Find users related by shared risk signals",
		Long: `Find users related by shared IP address, payment address, or card
fingerprint. By default the server evaluates all available signals; repeat
--signal to restrict the search to one or more signals.`,
		Example: `  gumroad admin users related --user-id 2245593582708
  gumroad admin users related --email user@example.com --signal ip --signal payment_address
  gumroad admin users related --email user@example.com --limit 25 --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}
			normalizedSignals, err := normalizeRelatedSignals(c, signals)
			if err != nil {
				return err
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", limit); err != nil {
				return err
			}

			params := target.values()
			applyRelatedParams(params, normalizedSignals, limit, c.Flags().Changed("limit"))

			return admincmd.RunGetDecoded[relatedResponse](opts, "Fetching related users...", "/users/related", params, func(resp relatedResponse) error {
				return renderRelated(opts, target.identifier(), resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)
	cmd.Flags().StringArrayVar(&signals, "signal", nil, "Related signal to evaluate: ip, payment_address, card_fingerprint (repeatable)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum related users per signal (default 50, capped server-side at 50)")

	return cmd
}

func applyRelatedParams(params url.Values, signals []string, limit int, limitChanged bool) {
	if len(signals) > 0 {
		params.Set("signals", strings.Join(signals, ","))
	}
	if limitChanged {
		params.Set("limit", strconv.Itoa(limit))
	}
}

func normalizeRelatedSignals(cmd *cobra.Command, signals []string) ([]string, error) {
	out := make([]string, 0, len(signals))
	for _, signal := range signals {
		normalized := strings.TrimSpace(signal)
		if normalized == "" || !validRelatedSignals[normalized] {
			return nil, cmdutil.UsageErrorf(cmd, "--signal must be one of: %s", strings.Join(relatedSignalOrder, ", "))
		}
		out = append(out, normalized)
	}
	return out, nil
}

func renderRelated(opts cmdutil.Options, identifier string, resp relatedResponse) error {
	if opts.PlainOutput {
		return writeRelatedPlain(opts.Out(), resp.RelatedUsers)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		headline := fmt.Sprintf("%d related user(s) for %s", len(resp.RelatedUsers), identifier)
		if err := output.Writeln(w, style.Bold(headline)); err != nil {
			return err
		}
		if resp.UserID != "" && resp.UserID != identifier {
			if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
				return err
			}
		}
		if err := output.Writef(w, "Signals evaluated: %s\n", relatedListLabel(resp.SignalsEvaluated)); err != nil {
			return err
		}
		if err := output.Writef(w, "Per-signal limit: %d\n", resp.PerSignalLimit); err != nil {
			return err
		}

		if len(resp.RelatedUsers) == 0 {
			if err := output.Writeln(w, ""); err != nil {
				return err
			}
			if err := output.Writef(w, "No related users found for %s.\n", identifier); err != nil {
				return err
			}
			return writeRelatedCapWarning(w, style, resp.Truncated)
		}

		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		if err := writeRelatedTable(w, style, resp.RelatedUsers); err != nil {
			return err
		}
		return writeRelatedCapWarning(w, style, resp.Truncated)
	})
}

func writeRelatedPlain(w io.Writer, users []relatedUser) error {
	rows := make([][]string, 0, len(users))
	for _, user := range users {
		rows = append(rows, []string{
			user.ID,
			user.Email,
			user.Name,
			relatedRiskLabel(user.RiskState),
			relatedRelationsLabel(user.Relations),
		})
	}
	return output.PrintPlain(w, rows)
}

func writeRelatedTable(w io.Writer, style output.Styler, users []relatedUser) error {
	tbl := output.NewStyledTable(style, "ID", "EMAIL", "NAME", "RISK", "RELATIONS")
	for _, user := range users {
		tbl.AddRow(
			user.ID,
			user.Email,
			user.Name,
			relatedRiskLabel(user.RiskState),
			relatedRelationsLabel(user.Relations),
		)
	}
	return tbl.Render(w)
}

func relatedRiskLabel(risk riskState) string {
	return fallback(risk.Status, risk.UserRiskState)
}

func relatedRelationsLabel(relations []relatedRelation) string {
	labels := make([]string, 0, len(relations))
	for _, relation := range relations {
		labels = append(labels, relatedRelationLabel(relation))
	}
	return strings.Join(labels, ", ")
}

func relatedRelationLabel(relation relatedRelation) string {
	label := relation.Signal
	if relation.SharedValue != "" {
		label += ":" + relation.SharedValue
	}
	if len(relation.Via) > 0 {
		label += " (" + strings.Join(relation.Via, ", ") + ")"
	}
	return label
}

func relatedListLabel(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func writeRelatedCapWarning(w io.Writer, style output.Styler, truncated map[string]bool) error {
	signals := truncatedRelatedSignals(truncated)
	if len(signals) == 0 {
		return nil
	}
	return output.Writef(w, "\n%s results capped for signals: %s.\n", style.Yellow("Warning:"), strings.Join(signals, ", "))
}

func truncatedRelatedSignals(truncated map[string]bool) []string {
	if len(truncated) == 0 {
		return nil
	}

	signals := make([]string, 0, len(truncated))
	seen := make(map[string]bool, len(truncated))
	for _, signal := range relatedSignalOrder {
		if truncated[signal] {
			signals = append(signals, signal)
			seen[signal] = true
		}
	}

	extras := make([]string, 0)
	for signal, capped := range truncated {
		if capped && !seen[signal] {
			extras = append(extras, signal)
		}
	}
	sort.Strings(extras)
	return append(signals, extras...)
}
