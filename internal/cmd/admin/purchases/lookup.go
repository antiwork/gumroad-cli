package purchases

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type lookupResponse struct {
	Lookup     lookupInfo        `json:"lookup"`
	Purchases  []purchase        `json:"purchases"`
	Pagination cursor.Pagination `json:"pagination"`
}

type lookupInfo struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type lookupFlags struct {
	StripeFingerprint string
	BrowserGUID       string
	IPAddress         string
}

type lookupSignal struct {
	flag  string
	field string
	value string
}

func newLookupCmd() *cobra.Command {
	var (
		signals lookupFlags
		page    cursor.Flags
	)

	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Look up purchases sharing a fingerprint, browser, or IP",
		Example: `  gumroad admin purchases lookup --stripe-fingerprint fp_abc
  gumroad admin purchases lookup --browser-guid bguid_abc
  gumroad admin purchases lookup --ip-address 1.2.3.4 --limit 25
  gumroad admin purchases lookup --ip-address 1.2.3.4 --cursor cur-next`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			signal, err := resolveLookupSignal(c, signals)
			if err != nil {
				return err
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", page.Limit); err != nil {
				return err
			}

			params := url.Values{}
			params.Set(signal.field, signal.value)
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[lookupResponse](opts, "Looking up purchases...", "/purchases/lookup", params, func(resp lookupResponse) error {
				return renderLookup(opts, signal, resp)
			})
		},
	}

	cmd.Flags().StringVar(&signals.StripeFingerprint, "stripe-fingerprint", "", "Stripe card fingerprint to match")
	cmd.Flags().StringVar(&signals.BrowserGUID, "browser-guid", "", "Browser GUID to match")
	cmd.Flags().StringVar(&signals.IPAddress, "ip-address", "", "IP address to match")
	cursor.AddFlags(cmd, &page)

	return cmd
}

func resolveLookupSignal(cmd *cobra.Command, flags lookupFlags) (lookupSignal, error) {
	candidates := []lookupSignal{
		{flag: "stripe-fingerprint", field: "stripe_fingerprint", value: flags.StripeFingerprint},
		{flag: "browser-guid", field: "browser_guid", value: flags.BrowserGUID},
		{flag: "ip-address", field: "ip_address", value: flags.IPAddress},
	}

	var selected lookupSignal
	changedCount := 0
	for i := range candidates {
		candidate := candidates[i]
		if !cmd.Flags().Changed(candidate.flag) {
			continue
		}
		changedCount++
		selected = candidate
	}

	if changedCount != 1 {
		return lookupSignal{}, lookupSignalCountError(cmd)
	}

	selected.value = strings.TrimSpace(selected.value)
	if selected.value == "" {
		return lookupSignal{}, cmdutil.UsageErrorf(cmd, "--%s cannot be empty", selected.flag)
	}
	return selected, nil
}

func lookupSignalCountError(cmd *cobra.Command) error {
	return cmdutil.UsageErrorf(cmd, "exactly one of --stripe-fingerprint, --browser-guid, or --ip-address must be provided")
}

func renderLookup(opts cmdutil.Options, signal lookupSignal, resp lookupResponse) error {
	if opts.PlainOutput {
		return writeLookupPlain(opts.Out(), resp.Purchases)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		label := lookupLabel(signal, resp.Lookup)
		if len(resp.Purchases) == 0 {
			if err := output.Writef(w, "No purchases found for %s.\n", label); err != nil {
				return err
			}
			return cursor.WriteMoreFooter(w, resp.Pagination)
		}

		if err := output.Writeln(w, style.Bold(fmt.Sprintf("%d purchase(s) for %s", len(resp.Purchases), label))); err != nil {
			return err
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		if err := writeLookupTable(w, style, resp.Purchases); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writeLookupPlain(w io.Writer, purchases []purchase) error {
	rows := make([][]string, 0, len(purchases))
	for _, p := range purchases {
		rows = append(rows, lookupRow(p))
	}
	return output.PrintPlain(w, rows)
}

func writeLookupTable(w io.Writer, style output.Styler, purchases []purchase) error {
	tbl := output.NewStyledTable(style, "ID", "BUYER", "SELLER", "PRODUCT", "AMOUNT", "STATE", "CREATED")
	for _, p := range purchases {
		tbl.AddRow(lookupRow(p)...)
	}
	return tbl.Render(w)
}

func lookupRow(p purchase) []string {
	return []string{
		p.ID,
		buyerLabel(p),
		sellerLabel(p),
		productLabel(p),
		amountLabel(p),
		statusLabel(p),
		p.CreatedAt,
	}
}

func lookupLabel(signal lookupSignal, info lookupInfo) string {
	field := info.Field
	if field == "" {
		field = signal.field
	}
	value := info.Value
	if value == "" {
		value = signal.value
	}
	if field == "" {
		return value
	}
	return fmt.Sprintf("%s=%s", field, value)
}
