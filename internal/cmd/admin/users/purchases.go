package users

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	adminpurchases "github.com/antiwork/gumroad-cli/internal/cmd/admin/purchases"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type userPurchasesResponse struct {
	UserID     string                    `json:"user_id"`
	Purchases  []adminpurchases.Purchase `json:"purchases"`
	Pagination cursor.Pagination         `json:"pagination"`
}

func newPurchasesCmd() *cobra.Command {
	var (
		lookup               userLookupFlags
		page                 cursor.Flags
		statuses             []string
		startAt              string
		endAt                string
		stripeFingerprint    string
		ipAddress            string
		chargedback          bool
		hasEarlyFraudWarning bool
		hasAffiliate         bool
	)

	cmd := &cobra.Command{
		Use:   "purchases",
		Short: "List purchases for a user",
		Long: `List a user's purchase history across sellers.

Status filters are sent as purchase_state values. Use --chargedback,
--has-early-fraud-warning, and --has-affiliate for the boolean risk filters.`,
		Example: `  gumroad admin users purchases --user-id 2245593582708
  gumroad admin users purchases --username sellerone
  gumroad admin users purchases --email buyer@example.com --status successful --status failed
  gumroad admin users purchases --user-id 2245593582708 --start-at 2026-01-01T00:00:00Z
  gumroad admin users purchases --user-id 2245593582708 --chargedback=false --limit 50`,
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

			params := target.Values()
			if err := applyUserPurchaseFilters(c, params, statuses, startAt, endAt, stripeFingerprint, ipAddress, chargedback, hasEarlyFraudWarning, hasAffiliate); err != nil {
				return err
			}
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[userPurchasesResponse](opts, "Fetching user purchases...", "/users/purchases", params, func(resp userPurchasesResponse) error {
				return renderUserPurchases(opts, target.Identifier(), resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)
	cmd.Flags().StringArrayVar(&statuses, "status", nil, "Purchase state filter (repeatable)")
	cmd.Flags().StringVar(&startAt, "start-at", "", "Only include purchases created at or after this ISO 8601 timestamp")
	cmd.Flags().StringVar(&endAt, "end-at", "", "Only include purchases created at or before this ISO 8601 timestamp")
	cmd.Flags().StringVar(&stripeFingerprint, "stripe-fingerprint", "", "Filter by Stripe card fingerprint")
	cmd.Flags().StringVar(&ipAddress, "ip-address", "", "Filter by purchase IP address")
	cmd.Flags().BoolVar(&chargedback, "chargedback", false, "Filter by whether the purchase has a chargeback")
	cmd.Flags().BoolVar(&hasEarlyFraudWarning, "has-early-fraud-warning", false, "Filter by whether the purchase has an early fraud warning")
	cmd.Flags().BoolVar(&hasAffiliate, "has-affiliate", false, "Filter by whether the purchase has affiliate credit")
	cursor.AddFlags(cmd, &page)

	return cmd
}

func applyUserPurchaseFilters(cmd *cobra.Command, params url.Values, statuses []string, startAt, endAt, stripeFingerprint, ipAddress string, chargedback, hasEarlyFraudWarning, hasAffiliate bool) error {
	normalizedStatuses, err := normalizePurchaseStatuses(cmd, statuses)
	if err != nil {
		return err
	}
	if len(normalizedStatuses) > 0 {
		// /users/purchases accepts a CSV status param; repeated CLI flags stay user-facing only.
		params.Set("status", strings.Join(normalizedStatuses, ","))
	}
	if startAt != "" {
		params.Set("start_at", startAt)
	}
	if endAt != "" {
		params.Set("end_at", endAt)
	}
	if stripeFingerprint != "" {
		params.Set("stripe_fingerprint", stripeFingerprint)
	}
	if ipAddress != "" {
		params.Set("ip_address", ipAddress)
	}
	if cmd.Flags().Changed("chargedback") {
		params.Set("chargedback", strconv.FormatBool(chargedback))
	}
	if cmd.Flags().Changed("has-early-fraud-warning") {
		params.Set("has_early_fraud_warning", strconv.FormatBool(hasEarlyFraudWarning))
	}
	if cmd.Flags().Changed("has-affiliate") {
		params.Set("has_affiliate", strconv.FormatBool(hasAffiliate))
	}
	return nil
}

func normalizePurchaseStatuses(cmd *cobra.Command, statuses []string) ([]string, error) {
	normalized := make([]string, 0, len(statuses))
	for _, status := range statuses {
		value := strings.TrimSpace(status)
		if value == "" {
			return nil, cmdutil.UsageErrorf(cmd, "--status cannot be empty")
		}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func renderUserPurchases(opts cmdutil.Options, identifier string, resp userPurchasesResponse) error {
	if opts.PlainOutput {
		return writeUserPurchasesPlain(opts.Out(), resp.Purchases)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.Purchases) == 0 {
			if err := output.Writef(w, "No purchases found for %s.\n", identifier); err != nil {
				return err
			}
			return cursor.WriteMoreFooter(w, resp.Pagination)
		}

		if err := output.Writeln(w, style.Bold(fmt.Sprintf("%d purchase(s) for %s", len(resp.Purchases), identifier))); err != nil {
			return err
		}
		if resp.UserID != "" && resp.UserID != identifier {
			if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
				return err
			}
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		if err := writeUserPurchasesTable(w, style, resp.Purchases); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writeUserPurchasesPlain(w io.Writer, purchases []adminpurchases.Purchase) error {
	rows := make([][]string, 0, len(purchases))
	for _, p := range purchases {
		rows = append(rows, purchaseRow(p))
	}
	return output.PrintPlain(w, rows)
}

func writeUserPurchasesTable(w io.Writer, style output.Styler, purchases []adminpurchases.Purchase) error {
	tbl := output.NewStyledTable(style, "ID", "SELLER", "PRODUCT", "AMOUNT", "STATE", "FLAGS", "CREATED")
	for _, p := range purchases {
		tbl.AddRow(purchaseRow(p)...)
	}
	return tbl.Render(w)
}

func purchaseRow(p adminpurchases.Purchase) []string {
	return []string{
		p.ID,
		adminpurchases.SellerLabel(p),
		adminpurchases.ProductLabel(p),
		adminpurchases.AmountLabel(p),
		adminpurchases.StatusLabel(p),
		adminpurchases.RiskFlagsLabel(p),
		p.CreatedAt,
	}
}
