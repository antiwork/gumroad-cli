package users

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const refundBalanceConfirmationMessage = "Refund %d unpaid purchase(s) totaling %s for user_id %s? This queues refunds to buyers and cannot be undone once processors accept them."

type unpaidBalanceResponse struct {
	Success          bool   `json:"success"`
	UserID           string `json:"user_id"`
	Count            int    `json:"count"`
	Total            int    `json:"total"`
	TotalAmountCents int    `json:"total_amount_cents"`
	Currency         string `json:"currency"`
}

type refundBalanceRequest struct {
	UserID                   string `json:"user_id"`
	ExpectedEmail            string `json:"expected_email"`
	ExpectedPurchaseCount    int    `json:"expected_purchase_count"`
	ExpectedTotalAmountCents int    `json:"expected_total_amount_cents"`
}

type refundBalanceResponse struct {
	Success          bool   `json:"success"`
	UserID           string `json:"user_id"`
	Status           string `json:"status"`
	Message          string `json:"message"`
	Count            int    `json:"count"`
	Total            int    `json:"total"`
	TotalAmountCents int    `json:"total_amount_cents"`
	Currency         string `json:"currency"`
}

func newRefundBalanceCmd() *cobra.Command {
	var targetFlags userMutationFlags

	cmd := &cobra.Command{
		Use:   "refund-balance",
		Short: "Refund unpaid purchases for a suspended user",
		Long: `Preview and queue refunds for all refundable purchases still in Gumroad-held unpaid balance.

The command first reads /users/unpaid_balance, then sends the returned count and
total cents back to /users/refund_balance as stale-state guards. --expected-email
is required because this is an irreversible money-moving workflow.

Pass --dry-run to run the preview GET and print the guarded POST that would be
sent without queueing refunds.`,
		Example: `  gumroad admin users refund-balance --user-id 2245593582708 --expected-email seller@example.com --dry-run
  gumroad admin users refund-balance --user-id 2245593582708 --expected-email seller@example.com --yes`,
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

			path := "users/refund_balance"

			if opts.DryRun {
				preview, err := fetchRefundBalanceDryRunPreview(opts, target.UserID)
				if err != nil {
					return err
				}
				req := refundBalanceRequest{
					UserID:                   target.UserID,
					ExpectedEmail:            target.ExpectedEmail,
					ExpectedPurchaseCount:    preview.Count,
					ExpectedTotalAmountCents: preview.TotalAmountCents,
				}
				if err := renderRefundBalancePreview(opts, fallback(preview.UserID, target.UserID), preview); err != nil {
					return err
				}
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), refundBalanceDryRunParams(req))
			}

			info, err := admincmd.ResolveMutationToken(opts)
			if err != nil {
				return err
			}
			if err := admincmd.WriteActorBanner(opts, info); err != nil {
				return err
			}
			client := admincmd.NewAPIClient(opts, info.Value)
			preview, err := fetchRefundBalancePreview(opts, client, target.UserID)
			if err != nil {
				return err
			}
			req := refundBalanceRequest{
				UserID:                   target.UserID,
				ExpectedEmail:            target.ExpectedEmail,
				ExpectedPurchaseCount:    preview.Count,
				ExpectedTotalAmountCents: preview.TotalAmountCents,
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(
				refundBalanceConfirmationMessage,
				preview.Count,
				formatRefundBalanceAmount(preview.TotalAmountCents, preview.Currency),
				target.UserID,
			))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "refund unpaid balance for user_id "+target.UserID, target.UserID)
			}

			data, err := postRefundBalance(opts, client, path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[refundBalanceResponse](data)
			if err != nil {
				return err
			}
			return renderRefundBalanceResult(opts, fallback(decoded.UserID, target.UserID), decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)

	return cmd
}

func fetchRefundBalanceDryRunPreview(opts cmdutil.Options, userID string) (unpaidBalanceResponse, error) {
	return admincmd.FetchGetDecoded[unpaidBalanceResponse](opts, "Checking unpaid balance...", "users/unpaid_balance", refundBalancePreviewParams(userID))
}

func fetchRefundBalancePreview(opts cmdutil.Options, client *adminapi.Client, userID string) (unpaidBalanceResponse, error) {
	data, err := runRefundBalanceAdminRequest(opts, "Checking unpaid balance...", func() (json.RawMessage, error) {
		return client.Get("users/unpaid_balance", refundBalancePreviewParams(userID))
	})
	if err != nil {
		return unpaidBalanceResponse{}, err
	}
	return cmdutil.DecodeJSON[unpaidBalanceResponse](data)
}

func postRefundBalance(opts cmdutil.Options, client *adminapi.Client, path string, req refundBalanceRequest) (json.RawMessage, error) {
	return runRefundBalanceAdminRequest(opts, "Queueing refund balance...", func() (json.RawMessage, error) {
		return client.PostJSON(path, req)
	})
}

func runRefundBalanceAdminRequest(opts cmdutil.Options, spinnerMessage string, run func() (json.RawMessage, error)) (json.RawMessage, error) {
	if cmdutil.ShouldShowSpinner(opts) {
		sp := output.NewSpinnerTo(spinnerMessage, opts.Err())
		sp.Start()
		defer sp.Stop()
	}
	return run()
}

func refundBalancePreviewParams(userID string) url.Values {
	params := url.Values{}
	params.Set("user_id", userID)
	return params
}

func refundBalanceDryRunParams(req refundBalanceRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	params.Set("expected_email", req.ExpectedEmail)
	params.Set("expected_purchase_count", strconv.Itoa(req.ExpectedPurchaseCount))
	params.Set("expected_total_amount_cents", strconv.Itoa(req.ExpectedTotalAmountCents))
	return params
}

func renderRefundBalancePreview(opts cmdutil.Options, userID string, preview unpaidBalanceResponse) error {
	if opts.UsesJSONOutput() || opts.PlainOutput || opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Refund balance preview")); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "User ID: %s\n", userID); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Purchases: %d\n", preview.Count); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Amount: %s\n", formatRefundBalanceAmount(preview.TotalAmountCents, preview.Currency)); err != nil {
		return err
	}
	if preview.Currency != "" {
		return output.Writef(opts.Out(), "Currency: %s\n", preview.Currency)
	}
	return nil
}

func renderRefundBalanceResult(opts cmdutil.Options, userID string, resp refundBalanceResponse) error {
	message := fallback(resp.Message, resp.Status)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			strconv.FormatBool(resp.Success),
			message,
			userID,
			resp.Status,
			strconv.Itoa(resp.Count),
			strconv.Itoa(resp.TotalAmountCents),
			resp.Currency,
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(message)); err != nil {
		return err
	}
	if err := cmdutil.WriteIdentifierLine(opts.Out(), "User ID", message, userID); err != nil {
		return err
	}
	if resp.Status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", resp.Status); err != nil {
			return err
		}
	}
	if err := output.Writef(opts.Out(), "Purchases: %d\n", resp.Count); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Amount: %s\n", formatRefundBalanceAmount(resp.TotalAmountCents, resp.Currency)); err != nil {
		return err
	}
	if resp.Currency != "" {
		return output.Writef(opts.Out(), "Currency: %s\n", resp.Currency)
	}
	return nil
}

func formatRefundBalanceAmount(cents int, currency string) string {
	if currency == "" {
		return fmt.Sprintf("%d cents", cents)
	}
	return fmt.Sprintf("%d %s cents", cents, strings.ToUpper(currency))
}
