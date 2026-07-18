package users

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const refundAllForFraudConfirmationMessage = "Queue fraud refunds for %d purchase(s) of user_id %s%s? This refunds every remaining sale, cancels linked subscriptions, and cannot be undone once queued."

type refundAllForFraudRequest struct {
	UserID                string `json:"user_id"`
	ExpectedEmail         string `json:"expected_email"`
	ExpectedPurchaseCount int    `json:"expected_purchase_count"`
	BlockBuyers           bool   `json:"block_buyers,omitempty"`
}

type refundAllForFraudResponse struct {
	Success           bool   `json:"success"`
	UserID            string `json:"user_id"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	PurchasesToRefund int    `json:"purchases_to_refund"`
	BlockBuyers       bool   `json:"block_buyers"`
}

func newRefundAllForFraudCmd() *cobra.Command {
	var (
		targetFlags   userMutationFlags
		expectedCount int
		blockBuyers   bool
	)

	cmd := &cobra.Command{
		Use:   "refund-all-for-fraud",
		Short: "Queue fraud refunds for every remaining purchase of a suspended seller",
		Long: `Queue refunds for all successful, non-refunded purchases of a suspended seller
as fraud. The server validates and enqueues a background job; refunds are
processed asynchronously with per-purchase retries. Completion is recorded as a
comment on the seller account and in the admin audit log.

By default buyers are NOT blocked (seller-fraud case: the buyers are victims).
Pass --block-buyers only when the buyers themselves are fraudulent (self-purchase
or card-testing ring); that variant also blocks each buyer's email, card
fingerprint, browser, and IP platform-wide.

--expected-email and --expected-count are required stale-state guards: the server
responds 409 if either does not match the current account state. The seller must
already be suspended; suspend first with 'gumroad admin users suspend'.

Requires the bulk endpoint POST /internal/admin/users/refund_all_for_fraud on
the Gumroad API.`,
		Example: `  gumroad admin users refund-all-for-fraud --user-id 2245593582708 --expected-email seller@example.com --expected-count 18 --dry-run
  gumroad admin users refund-all-for-fraud --user-id 2245593582708 --expected-email seller@example.com --expected-count 18 --yes
  gumroad admin users refund-all-for-fraud --user-id 2245593582708 --expected-email seller@example.com --expected-count 18 --block-buyers --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}
			if target.ExpectedEmail == "" {
				return cmdutil.MissingFlagError(c, "--expected-email")
			}
			if !c.Flags().Changed("expected-count") {
				return cmdutil.MissingFlagError(c, "--expected-count")
			}
			if expectedCount < 0 {
				return cmdutil.UsageErrorf(c, "--expected-count must be a non-negative integer")
			}

			req := refundAllForFraudRequest{
				UserID:                target.UserID,
				ExpectedEmail:         target.ExpectedEmail,
				ExpectedPurchaseCount: expectedCount,
				BlockBuyers:           blockBuyers,
			}
			path := "users/refund_all_for_fraud"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), refundAllForFraudDryRunParams(req))
			}

			blockingNote := ""
			if blockBuyers {
				blockingNote = ", blocking every buyer platform-wide"
			}
			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(refundAllForFraudConfirmationMessage, expectedCount, target.UserID, blockingNote))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "refund all purchases of user_id "+target.UserID+" for fraud", target.UserID)
			}

			data, err := admincmd.FetchPostJSON(opts, "Queueing bulk fraud refund...", path, req)
			if err != nil {
				return refundAllForFraudConflictError(err)
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[refundAllForFraudResponse](data)
			if err != nil {
				return err
			}
			return renderRefundAllForFraud(opts, fallback(decoded.UserID, target.UserID), decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().IntVar(&expectedCount, "expected-count", 0, "Expected number of refundable purchases (required; the server rejects a mismatch)")
	cmd.Flags().BoolVar(&blockBuyers, "block-buyers", false, "Also block every buyer platform-wide (only for buyer-fraud cases like card-testing rings)")

	return cmd
}

func refundAllForFraudConflictError(err error) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return err
	}
	return fmt.Errorf("%s — re-check the account with 'gumroad admin users info' and retry with current values; a 409 also means a bulk refund run may already be queued", apiErr.Message)
}

func refundAllForFraudDryRunParams(req refundAllForFraudRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	params.Set("expected_email", req.ExpectedEmail)
	params.Set("expected_purchase_count", strconv.Itoa(req.ExpectedPurchaseCount))
	if req.BlockBuyers {
		params.Set("block_buyers", "true")
	}
	return params
}

func renderRefundAllForFraud(opts cmdutil.Options, userID string, resp refundAllForFraudResponse) error {
	blocking := "buyers will not be blocked"
	if resp.BlockBuyers {
		blocking = "buyers will be blocked"
	}
	summary := fmt.Sprintf("Queued fraud refunds for %d purchase(s); %s", resp.PurchasesToRefund, blocking)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			strconv.FormatBool(resp.Success),
			fallback(resp.Message, resp.Status),
			userID,
			resp.Status,
			strconv.Itoa(resp.PurchasesToRefund),
			strconv.FormatBool(resp.BlockBuyers),
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(summary)); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "User ID: %s\n", userID); err != nil {
		return err
	}
	if resp.Status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", resp.Status); err != nil {
			return err
		}
	}
	return output.Writeln(opts.Out(), "Refunds run in the background with per-purchase retries; check the seller's comments and the admin audit log for completion.")
}
