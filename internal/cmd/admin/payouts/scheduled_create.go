package payouts

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type scheduledCreateRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
	Processor     string `json:"processor"`
	PayoutDate    string `json:"payout_date,omitempty"`
	Note          string `json:"note,omitempty"`
}

type scheduledCreateResponse struct {
	Success         bool            `json:"success"`
	UserID          string          `json:"user_id"`
	Message         string          `json:"message"`
	ScheduledPayout scheduledPayout `json:"scheduled_payout"`
}

func newScheduledCreateCmd() *cobra.Command {
	var (
		targetFlags mutationFlags
		processor   string
		payoutDate  string
		note        string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a delayed scheduled payout for a suspended user",
		Long: `Create a ScheduledPayout row for a suspended user with unpaid balance.

The server defaults --payout-date to 21 days from the current UTC date. Pass a
YYYY-MM-DD date to schedule a specific UTC payout date. --processor is required
and must be stripe or paypal.

This schedules money movement. --yes is required.`,
		Example: `  gumroad admin payouts scheduled create --user-id 2245593582708 --processor stripe --yes
  gumroad admin payouts scheduled create --user-id 2245593582708 --expected-email seller@example.com --processor paypal --payout-date 2026-06-15 --note "Appeal window closes before payout." --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}
			if processor == "" {
				return cmdutil.MissingFlagError(c, "--processor")
			}
			normalizedProcessor := strings.ToLower(strings.TrimSpace(processor))
			if normalizedProcessor != "stripe" && normalizedProcessor != "paypal" {
				return cmdutil.UsageErrorf(c, "--processor must be one of: stripe, paypal")
			}
			if err := cmdutil.RequireDateFlag(c, "payout-date", payoutDate); err != nil {
				return err
			}

			req := scheduledCreateRequest{
				UserID:        target.UserID,
				ExpectedEmail: target.ExpectedEmail,
				Processor:     normalizedProcessor,
				PayoutDate:    payoutDate,
				Note:          note,
			}

			confirmMsg := "Schedule " + normalizedProcessor + " payout for user_id " + target.UserID + "? This schedules money movement."
			if payoutDate != "" {
				confirmMsg = "Schedule " + normalizedProcessor + " payout for user_id " + target.UserID + " on " + payoutDate + "? This schedules money movement."
			}
			ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "schedule payout for user_id "+target.UserID, target.UserID)
			}

			path := "scheduled_payouts"
			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), scheduledCreateDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Creating scheduled payout...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[scheduledCreateResponse](data)
			if err != nil {
				return err
			}
			return renderScheduledCreate(opts, fallbackStr(decoded.UserID, target.UserID), decoded)
		},
	}

	addMutationFlags(cmd, &targetFlags)
	cmd.Flags().StringVar(&processor, "processor", "", "Payout processor: stripe or paypal (required)")
	cmd.Flags().StringVar(&payoutDate, "payout-date", "", "UTC payout date in YYYY-MM-DD (defaults server-side to today + 21 days)")
	cmd.Flags().StringVar(&note, "note", "", "Optional payout note recorded on the user")

	return cmd
}

func scheduledCreateDryRunParams(req scheduledCreateRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	params.Set("processor", req.Processor)
	if req.PayoutDate != "" {
		params.Set("payout_date", req.PayoutDate)
	}
	if req.Note != "" {
		params.Set("note", req.Note)
	}
	return params
}

func renderScheduledCreate(opts cmdutil.Options, userID string, resp scheduledCreateResponse) error {
	headline := fallbackStr(resp.Message, "Scheduled payout created")
	payout := resp.ScheduledPayout

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			"true",
			headline,
			userID,
			payout.ExternalID,
			formatScheduledAmount(payout),
			payout.Status,
			payout.ScheduledAt,
			payout.Processor,
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(headline)); err != nil {
		return err
	}
	if err := writeUserIDLine(opts.Out(), headline, userID); err != nil {
		return err
	}
	if payout.ExternalID != "" {
		if err := output.Writef(opts.Out(), "Payout ID: %s\n", payout.ExternalID); err != nil {
			return err
		}
	}
	if payout.AmountCents != 0 {
		if err := output.Writef(opts.Out(), "Amount: %s\n", formatScheduledAmount(payout)); err != nil {
			return err
		}
	}
	if payout.Status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", payout.Status); err != nil {
			return err
		}
	}
	if payout.ScheduledAt != "" {
		if err := output.Writef(opts.Out(), "Scheduled: %s\n", payout.ScheduledAt); err != nil {
			return err
		}
	}
	if payout.Processor != "" {
		return output.Writef(opts.Out(), "Processor: %s\n", payout.Processor)
	}
	return nil
}
