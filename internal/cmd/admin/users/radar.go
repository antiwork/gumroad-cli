package users

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const maxRadarLimit = 100

type radarResponse struct {
	UserID     string            `json:"user_id"`
	RadarStats radarStats        `json:"radar_stats"`
	RecentEFWs []recentEFW       `json:"recent_efws"`
	Pagination cursor.Pagination `json:"pagination"`
}

type radarStats struct {
	SuccessfulPurchases api.JSONInt            `json:"successful_purchases"`
	EFWCount            api.JSONInt            `json:"efw_count"`
	EFWByFraudType      map[string]api.JSONInt `json:"efw_by_fraud_type"`
	EFWWithElevatedRisk api.JSONInt            `json:"efw_with_elevated_risk"`
	EFWWithHighestRisk  api.JSONInt            `json:"efw_with_highest_risk"`
	DisputeCount        api.JSONInt            `json:"dispute_count"`
	DisputeRate         float64                `json:"dispute_rate"`
}

type recentEFW struct {
	PurchaseID      string `json:"purchase_id"`
	FraudType       string `json:"fraud_type"`
	ChargeRiskLevel string `json:"charge_risk_level"`
	Resolution      string `json:"resolution"`
	CreatedAt       string `json:"created_at"`
}

func newRadarCmd() *cobra.Command {
	var (
		lookup userLookupFlags
		page   cursor.Flags
	)

	cmd := &cobra.Command{
		Use:   "radar",
		Short: "View seller-level Radar and early fraud warning stats",
		Long: `View seller-level Radar aggregates and recent early fraud warnings.

Identify the seller with --email or --user-id. When both are supplied, the
server resolves by --user-id.`,
		Example: `  gumroad admin users radar --user-id 2245593582708
  gumroad admin users radar --email seller@example.com --limit 50
  gumroad admin users radar --user-id 2245593582708 --cursor cur-next --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", page.Limit); err != nil {
				return err
			}
			if c.Flags().Changed("limit") && page.Limit > maxRadarLimit {
				return cmdutil.UsageErrorf(c, "--limit must be %d or less", maxRadarLimit)
			}

			params := target.Values()
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[radarResponse](opts, "Fetching Radar stats...", "/users/radar_stats", params, func(resp radarResponse) error {
				return renderRadar(opts, target.Identifier(), resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)
	cursor.AddFlags(cmd, &page, cursor.Options{LimitUsage: "Maximum recent EFWs to return (default 20, capped at 100)"})

	return cmd
}

func renderRadar(opts cmdutil.Options, identifier string, resp radarResponse) error {
	if opts.PlainOutput {
		return writeRadarPlain(opts.Out(), identifier, resp)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writeRadarStatsBlock(w, style, identifier, resp.UserID, resp.RadarStats); err != nil {
			return err
		}
		if len(resp.RecentEFWs) == 0 {
			if err := output.Writeln(w, "Recent EFWs: none"); err != nil {
				return err
			}
			return cursor.WriteMoreFooter(w, resp.Pagination)
		}
		if err := output.Writeln(w, style.Bold("Recent EFWs:")); err != nil {
			return err
		}
		if err := writeRadarEFWTable(w, style, resp.RecentEFWs); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writeRadarStatsBlock(w io.Writer, style output.Styler, identifier, userID string, stats radarStats) error {
	if err := output.Writeln(w, style.Bold("Radar stats for "+identifier)); err != nil {
		return err
	}
	if userID != "" && userID != identifier {
		if err := output.Writef(w, "User ID: %s\n", userID); err != nil {
			return err
		}
	}

	rows := []struct {
		label string
		value string
	}{
		{"Successful purchases", formatRadarInt(stats.SuccessfulPurchases)},
		{"Early fraud warnings", formatRadarInt(stats.EFWCount)},
		{"EFW by fraud type", radarFraudTypeCountsLabel(stats.EFWByFraudType)},
		{"Elevated risk EFWs", formatRadarInt(stats.EFWWithElevatedRisk)},
		{"Highest risk EFWs", formatRadarInt(stats.EFWWithHighestRisk)},
		{"Disputes", formatRadarInt(stats.DisputeCount)},
		{"Dispute rate", formatRadarDecimal(stats.DisputeRate) + "%"},
	}
	for _, row := range rows {
		if err := output.Writef(w, "%s: %s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return output.Writeln(w, "")
}

func writeRadarEFWTable(w io.Writer, style output.Styler, efws []recentEFW) error {
	tbl := output.NewStyledTable(style, "PURCHASE/CHARGE", "FRAUD TYPE", "RISK", "RESOLUTION", "CREATED")
	for _, efw := range efws {
		tbl.AddRow(radarEFWRow(efw)...)
	}
	return tbl.Render(w)
}

func writeRadarPlain(w io.Writer, identifier string, resp radarResponse) error {
	capacity := len(resp.RecentEFWs)
	if capacity == 0 {
		capacity = 1
	}
	rows := make([][]string, 0, capacity)
	if len(resp.RecentEFWs) == 0 {
		rows = append(rows, radarPlainRow(identifier, resp.UserID, resp.RadarStats, recentEFW{}))
	} else {
		for _, efw := range resp.RecentEFWs {
			rows = append(rows, radarPlainRow(identifier, resp.UserID, resp.RadarStats, efw))
		}
	}
	return output.PrintPlain(w, rows)
}

func radarPlainRow(identifier, userID string, stats radarStats, efw recentEFW) []string {
	return append([]string{
		identifier,
		userID,
		formatRadarInt(stats.SuccessfulPurchases),
		formatRadarInt(stats.EFWCount),
		radarFraudTypeCountsLabel(stats.EFWByFraudType),
		formatRadarInt(stats.EFWWithElevatedRisk),
		formatRadarInt(stats.EFWWithHighestRisk),
		formatRadarInt(stats.DisputeCount),
		formatRadarDecimal(stats.DisputeRate),
	}, radarEFWRow(efw)...)
}

func radarEFWRow(efw recentEFW) []string {
	return []string{
		efw.PurchaseID,
		efw.FraudType,
		efw.ChargeRiskLevel,
		efw.Resolution,
		efw.CreatedAt,
	}
}

func radarFraudTypeCountsLabel(counts map[string]api.JSONInt) string {
	if len(counts) == 0 {
		return "(none)"
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, formatRadarInt(counts[key])))
	}
	return strings.Join(parts, ", ")
}

func formatRadarInt(value api.JSONInt) string {
	return strconv.Itoa(int(value))
}

func formatRadarDecimal(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 2, 64)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if formatted == "" || formatted == "-0" {
		return "0"
	}
	return formatted
}
