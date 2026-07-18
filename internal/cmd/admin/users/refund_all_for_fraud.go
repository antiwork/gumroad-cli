package users

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const refundAllForFraudConfirmationMessage = "Refund ALL successful purchases of user_id %s for fraud and block every buyer? This refunds every remaining sale, cancels linked subscriptions, and cannot be undone."

type refundAllForFraudRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
	Force         bool   `json:"force,omitempty"`
}

type refundAllForFraudFailure struct {
	PurchaseExternalIDNumeric int64  `json:"purchase_external_id_numeric"`
	Error                     string `json:"error"`
}

type refundAllForFraudResponse struct {
	Success       bool                       `json:"success"`
	UserID        string                     `json:"user_id"`
	Message       string                     `json:"message"`
	RefundedCount int                        `json:"refunded_count"`
	SkippedCount  int                        `json:"skipped_count"`
	Failed        []refundAllForFraudFailure `json:"failed"`
}

func newRefundAllForFraudCmd() *cobra.Command {
	var (
		targetFlags userMutationFlags
		force       bool
	)

	cmd := &cobra.Command{
		Use:   "refund-all-for-fraud",
		Short: "Refund every successful purchase of a fraudulent seller",
		Long: `Refund all successful, non-refunded, non-charged-back purchases of a seller as
fraud in one call, blocking each buyer and cancelling linked subscriptions.

The seller must already be suspended or flagged; pass --force to override that
guard for a seller in good standing. Already-refunded purchases are skipped, so
re-running the command after a partial failure only retries what is left.

Requires the bulk endpoint POST /internal/admin/users/refund_all_for_fraud on
the Gumroad API.`,
		Example: `  gumroad admin users refund-all-for-fraud --user-id 2245593582708 --dry-run
  gumroad admin users refund-all-for-fraud --user-id 2245593582708 --yes
  gumroad admin users refund-all-for-fraud --user-id 2245593582708 --force --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			req := refundAllForFraudRequest{
				UserID:        target.UserID,
				ExpectedEmail: target.ExpectedEmail,
				Force:         force,
			}
			path := "users/refund_all_for_fraud"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), refundAllForFraudDryRunParams(req))
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(refundAllForFraudConfirmationMessage, target.UserID))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "refund all purchases of user_id "+target.UserID+" for fraud", target.UserID)
			}

			data, err := admincmd.FetchPostJSON(opts, "Refunding all purchases for fraud...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				if err := cmdutil.PrintJSONResponse(opts, data); err != nil {
					return err
				}
				return refundAllForFraudExitError(data)
			}

			decoded, err := cmdutil.DecodeJSON[refundAllForFraudResponse](data)
			if err != nil {
				return err
			}
			if err := renderRefundAllForFraud(opts, fallback(decoded.UserID, target.UserID), decoded); err != nil {
				return err
			}
			if len(decoded.Failed) > 0 {
				return fmt.Errorf("%d purchase(s) failed to refund; re-run the command to retry only the remaining purchases", len(decoded.Failed))
			}
			return nil
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().BoolVar(&force, "force", false, "Refund even when the seller is not suspended or flagged")

	return cmd
}

// In JSON output mode the raw response is printed as-is, but the process must
// still exit non-zero when any purchase failed so scripts can detect partial
// failures without parsing the payload.
func refundAllForFraudExitError(data json.RawMessage) error {
	decoded, err := cmdutil.DecodeJSON[refundAllForFraudResponse](data)
	if err != nil {
		return nil //nolint:nilerr // Output already printed; an unparseable body should not fail the command.
	}
	if len(decoded.Failed) > 0 {
		return fmt.Errorf("%d purchase(s) failed to refund; re-run the command to retry only the remaining purchases", len(decoded.Failed))
	}
	return nil
}

func refundAllForFraudDryRunParams(req refundAllForFraudRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	if req.Force {
		params.Set("force", "true")
	}
	return params
}

func renderRefundAllForFraud(opts cmdutil.Options, userID string, resp refundAllForFraudResponse) error {
	summary := fmt.Sprintf("Refunded %d, skipped %d already-refunded, %d failed", resp.RefundedCount, resp.SkippedCount, len(resp.Failed))

	if opts.PlainOutput {
		rows := [][]string{{
			// The API always reports success once the batch runs; whether
			// every purchase actually refunded is what callers care about.
			strconv.FormatBool(len(resp.Failed) == 0),
			summary,
			userID,
			strconv.Itoa(resp.RefundedCount),
			strconv.Itoa(resp.SkippedCount),
			strconv.Itoa(len(resp.Failed)),
		}}
		for _, failure := range resp.Failed {
			rows = append(rows, []string{"failed", strconv.FormatInt(failure.PurchaseExternalIDNumeric, 10), failure.Error})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	headline := "Refunded all remaining purchases for fraud and blocked the buyers"
	if len(resp.Failed) > 0 {
		headline = "Bulk fraud refund finished with failures"
	}
	styledHeadline := style.Green(headline)
	if len(resp.Failed) > 0 {
		styledHeadline = style.Red(headline)
	}
	if err := output.Writeln(opts.Out(), styledHeadline); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "User ID: %s\n", userID); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "%s\n", summary); err != nil {
		return err
	}
	if len(resp.Failed) == 0 {
		return nil
	}

	if err := output.Writeln(opts.Out(), ""); err != nil {
		return err
	}
	tbl := output.NewStyledTable(style, "PURCHASE ID", "ERROR")
	for _, failure := range resp.Failed {
		tbl.AddRow(strconv.FormatInt(failure.PurchaseExternalIDNumeric, 10), failure.Error)
	}
	return tbl.Render(opts.Out())
}
