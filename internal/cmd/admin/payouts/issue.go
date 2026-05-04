package payouts

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type issueRequest struct {
	Email                string `json:"email"`
	PayoutProcessor      string `json:"payout_processor"`
	PayoutPeriodEndDate  string `json:"payout_period_end_date"`
	ShouldSplitTheAmount bool   `json:"should_split_the_amount,omitempty"`
}

type issueResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Payout  issuePayout `json:"payout"`
}

type issuePayout struct {
	ExternalID  string      `json:"external_id"`
	AmountCents api.JSONInt `json:"amount_cents"`
	Currency    string      `json:"currency"`
	State       string      `json:"state"`
	Processor   string      `json:"processor"`
}

func newIssueCmd() *cobra.Command {
	var (
		email     string
		through   string
		processor string
		split     bool
	)

	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Issue a manual payout for a seller",
		Long: `Issue a manual payout for a seller through Stripe or PayPal.

The --through date is the payout period end date and must be in the past
(YYYY-MM-DD). --split is only valid with --processor paypal and asks the
server to split the payout amount across multiple PayPal transfers.

This moves money. --yes is required.`,
		Example: `  gumroad admin payouts issue --email seller@example.com --through 2026-04-30 --processor stripe --yes
  gumroad admin payouts issue --email seller@example.com --through 2026-04-30 --processor paypal --split --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}
			if through == "" {
				return cmdutil.MissingFlagError(c, "--through")
			}
			if processor == "" {
				return cmdutil.MissingFlagError(c, "--processor")
			}
			if err := cmdutil.RequireDateFlag(c, "through", through); err != nil {
				return err
			}
			normalizedProcessor := strings.ToLower(strings.TrimSpace(processor))
			if normalizedProcessor != "stripe" && normalizedProcessor != "paypal" {
				return cmdutil.UsageErrorf(c, "--processor must be one of: stripe, paypal")
			}
			if split && normalizedProcessor != "paypal" {
				return cmdutil.UsageErrorf(c, "--split requires --processor paypal")
			}
			if t, err := time.Parse("2006-01-02", through); err == nil {
				if !t.Before(time.Now().UTC().Truncate(24 * time.Hour)) {
					return cmdutil.UsageErrorf(c, "--through must be a date in the past")
				}
			}

			req := issueRequest{
				Email:                email,
				PayoutProcessor:      normalizedProcessor,
				PayoutPeriodEndDate:  through,
				ShouldSplitTheAmount: split,
			}

			confirmMsg := "Issue manual " + normalizedProcessor + " payout for " + email + " through " + through + "? This moves money."
			ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "issue payout for "+email, email)
			}

			path := "payouts/issue"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), issueDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Issuing payout...", path, req)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[issueResponse](data)
			if err != nil {
				return err
			}
			return renderIssue(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Seller email (required)")
	cmd.Flags().StringVar(&through, "through", "", "Payout period end date in YYYY-MM-DD (required, must be in the past)")
	cmd.Flags().StringVar(&processor, "processor", "", "Payout processor: stripe or paypal (required)")
	cmd.Flags().BoolVar(&split, "split", false, "Split the amount across multiple PayPal transfers (paypal only)")

	return cmd
}

func issueDryRunParams(req issueRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	params.Set("payout_processor", req.PayoutProcessor)
	params.Set("payout_period_end_date", req.PayoutPeriodEndDate)
	if req.ShouldSplitTheAmount {
		params.Set("should_split_the_amount", "true")
	}
	return params
}

func renderIssue(opts cmdutil.Options, email string, resp issueResponse) error {
	headline := fallbackStr(resp.Message, "Issued payout for "+email)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			"true",
			headline,
			email,
			resp.Payout.ExternalID,
			formatIssueAmount(resp.Payout),
			resp.Payout.State,
			resp.Payout.Processor,
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(headline)); err != nil {
		return err
	}
	if resp.Payout.ExternalID != "" {
		if err := output.Writef(opts.Out(), "Payout ID: %s\n", resp.Payout.ExternalID); err != nil {
			return err
		}
	}
	if amount := formatIssueAmount(resp.Payout); amount != "" {
		if err := output.Writef(opts.Out(), "Amount: %s\n", amount); err != nil {
			return err
		}
	}
	if resp.Payout.State != "" {
		if err := output.Writef(opts.Out(), "State: %s\n", resp.Payout.State); err != nil {
			return err
		}
	}
	if resp.Payout.Processor != "" {
		return output.Writef(opts.Out(), "Processor: %s\n", resp.Payout.Processor)
	}
	return nil
}

func formatIssueAmount(p issuePayout) string {
	if p.AmountCents == 0 && p.Currency == "" {
		return ""
	}
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		return fmt.Sprintf("%d cents", p.AmountCents)
	}
	return fmt.Sprintf("%d %s cents", p.AmountCents, strings.ToUpper(currency))
}

func fallbackStr(value, alt string) string {
	if value == "" {
		return alt
	}
	return value
}
