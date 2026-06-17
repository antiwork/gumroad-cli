package payouts

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type payoutsResponse struct {
	UserID               string            `json:"user_id"`
	RecentPayouts        []payout          `json:"recent_payouts"`
	Pagination           cursor.Pagination `json:"pagination"`
	NextPayoutDate       string            `json:"next_payout_date"`
	BalanceForNextPayout string            `json:"balance_for_next_payout"`
	PayoutNote           string            `json:"payout_note"`
}

type payout struct {
	ExternalID        string       `json:"external_id"`
	AmountCents       api.JSONInt  `json:"amount_cents"`
	Currency          string       `json:"currency"`
	State             string       `json:"state"`
	CreatedAt         string       `json:"created_at"`
	Processor         string       `json:"processor"`
	BankAccountVisual string       `json:"bank_account_visual"`
	PaypalEmail       string       `json:"paypal_email"`
	TraceID           string       `json:"trace_id"`
	StripeTransferID  string       `json:"stripe_transfer_id"`
	BankAccount       *bankAccount `json:"bank_account"`
}

type bankAccount struct {
	BankNumber            string `json:"bank_number"`
	AccountHolderFullName string `json:"account_holder_full_name"`
	AccountType           string `json:"account_type"`
	Currency              string `json:"currency"`
}

func (p payout) hasDetails() bool {
	return p.TraceID != "" || p.StripeTransferID != "" || p.BankAccount != nil
}

func newListCmd() *cobra.Command {
	var (
		lookup lookupFlags
		page   cursor.Flags
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent payouts for a user",
		Long: `List recent payouts for a user as a compact table (id, amount, state, date,
processor, destination).

Stripe and bank payouts also render a per-row detail block below the table with
the Stripe transfer id and, for bank payouts, the destination bank account
(routing/BIC number, account holder, account type, currency) so support can
confirm where money landed without opening the Stripe dashboard. PayPal and
debit-card payouts carry no bank account and are omitted from the detail block.

Identify the user with --email or --user-id.`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveLookupTarget(c, lookup)
			if err != nil {
				return err
			}

			params := url.Values{}
			if target.Email != "" {
				params.Set("email", target.Email)
			}
			if target.UserID != "" {
				params.Set("user_id", target.UserID)
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", page.Limit); err != nil {
				return err
			}
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[payoutsResponse](opts, "Fetching payouts...", "/payouts", params, func(resp payoutsResponse) error {
				return renderPayouts(opts, target.identifier(), resp)
			})
		},
	}

	addLookupFlags(cmd, &lookup)
	cursor.AddFlags(cmd, &page)

	return cmd
}

func renderPayouts(opts cmdutil.Options, identifier string, resp payoutsResponse) error {
	if opts.PlainOutput {
		return writePayoutsPlain(opts.Out(), identifier, resp)
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, style.Bold(identifier)); err != nil {
			return err
		}
		if resp.UserID != "" && resp.UserID != identifier {
			if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
				return err
			}
		}
		if resp.NextPayoutDate != "" {
			if err := output.Writef(w, "Next payout: %s\n", resp.NextPayoutDate); err != nil {
				return err
			}
		}
		if resp.BalanceForNextPayout != "" {
			if err := output.Writef(w, "Balance for next payout: %s\n", resp.BalanceForNextPayout); err != nil {
				return err
			}
		}
		if resp.PayoutNote != "" {
			if err := output.Writef(w, "Payout note: %s\n", resp.PayoutNote); err != nil {
				return err
			}
		}
		if len(resp.RecentPayouts) == 0 {
			return output.Writeln(w, "No recent payouts found.")
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		if err := writePayoutsTable(w, style, resp.RecentPayouts); err != nil {
			return err
		}
		if err := writePayoutDetails(w, resp.RecentPayouts); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writePayoutsPlain(w io.Writer, identifier string, resp payoutsResponse) error {
	if len(resp.RecentPayouts) == 0 {
		return output.PrintPlain(w, [][]string{{identifier, "", "", "", "", "", "", resp.NextPayoutDate, resp.BalanceForNextPayout, resp.PayoutNote, "", "", "", "", ""}})
	}

	rows := make([][]string, 0, len(resp.RecentPayouts))
	for _, p := range resp.RecentPayouts {
		bankNumber, accountHolder, accountType, bankCurrency := bankAccountFields(p.BankAccount)
		rows = append(rows, []string{
			identifier,
			p.ExternalID,
			formatAmount(p),
			p.State,
			p.CreatedAt,
			p.Processor,
			payoutDestination(p),
			resp.NextPayoutDate,
			resp.BalanceForNextPayout,
			resp.PayoutNote,
			p.StripeTransferID,
			bankNumber,
			accountHolder,
			accountType,
			bankCurrency,
		})
	}
	return output.PrintPlain(w, rows)
}

func bankAccountFields(ba *bankAccount) (bankNumber, accountHolder, accountType, currency string) {
	if ba == nil {
		return "", "", "", ""
	}
	return ba.BankNumber, ba.AccountHolderFullName, ba.AccountType, ba.Currency
}

func writePayoutsTable(w io.Writer, style output.Styler, payouts []payout) error {
	tbl := output.NewStyledTable(style, "ID", "AMOUNT", "STATE", "DATE", "PROCESSOR", "DESTINATION")
	for _, p := range payouts {
		tbl.AddRow(p.ExternalID, formatAmount(p), p.State, p.CreatedAt, p.Processor, payoutDestination(p))
	}
	return tbl.Render(w)
}

func writePayoutDetails(w io.Writer, payouts []payout) error {
	var b strings.Builder
	for _, p := range payouts {
		if !p.hasDetails() {
			continue
		}
		fmt.Fprintf(&b, "  %s\n", p.ExternalID)
		if p.TraceID != "" {
			fmt.Fprintf(&b, "    trace: %s\n", p.TraceID)
		}
		if p.StripeTransferID != "" {
			fmt.Fprintf(&b, "    stripe transfer: %s\n", p.StripeTransferID)
		}
		if ba := p.BankAccount; ba != nil {
			if ba.BankNumber != "" {
				fmt.Fprintf(&b, "    routing/BIC: %s\n", ba.BankNumber)
			}
			if ba.AccountHolderFullName != "" {
				fmt.Fprintf(&b, "    account holder: %s\n", ba.AccountHolderFullName)
			}
			if ba.AccountType != "" {
				fmt.Fprintf(&b, "    account type: %s\n", ba.AccountType)
			}
			if ba.Currency != "" {
				fmt.Fprintf(&b, "    currency: %s\n", strings.ToUpper(ba.Currency))
			}
		}
	}
	if b.Len() == 0 {
		return nil
	}
	if err := output.Writeln(w, ""); err != nil {
		return err
	}
	if err := output.Writeln(w, "Details:"); err != nil {
		return err
	}
	return output.Writef(w, "%s", b.String())
}

func formatAmount(p payout) string {
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		return fmt.Sprintf("%d cents", p.AmountCents)
	}
	return fmt.Sprintf("%d %s cents", p.AmountCents, strings.ToUpper(currency))
}

func payoutDestination(p payout) string {
	if p.BankAccountVisual != "" {
		return p.BankAccountVisual
	}
	return p.PaypalEmail
}
