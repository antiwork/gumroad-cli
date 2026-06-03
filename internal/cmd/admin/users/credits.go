package users

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const creditMaxCents = 100_000

type creditPayload struct {
	ID              string `json:"id"`
	AmountCents     int    `json:"amount_cents"`
	Reason          string `json:"reason"`
	CreditingUserID string `json:"crediting_user_id"`
	CreatedAt       string `json:"created_at"`
}

type addCreditRequest struct {
	UserID        string `json:"user_id"`
	AmountCents   int    `json:"amount_cents"`
	Reason        string `json:"reason"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

type addCreditResponse struct {
	Success bool          `json:"success"`
	UserID  string        `json:"user_id"`
	Credit  creditPayload `json:"credit"`
}

type creditsResponse struct {
	UserID     string            `json:"user_id"`
	Credits    []creditPayload   `json:"credits"`
	Pagination cursor.Pagination `json:"pagination"`
}

func newCreditsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credits",
		Short: "List and issue user credits",
		Example: `  gumroad admin users credits list --user-id 2245593582708
  gumroad admin users credits list --email seller@example.com --cursor cur-next
  gumroad admin users credits add --user-id 2245593582708 --amount-cents 1000 --reason "Goodwill for checkout bug" --dry-run
  gumroad admin users credits add --user-id 2245593582708 --expected-email seller@example.com --amount-cents 1000 --reason "Goodwill for checkout bug" --yes`,
	}

	cmd.AddCommand(newCreditsListCmd())
	cmd.AddCommand(newCreditsAddCmd())

	return cmd
}

func newCreditsAddCmd() *cobra.Command {
	var (
		targetFlags userMutationFlags
		amountCents int
		reason      string
		allowLarge  bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Issue a goodwill account credit",
		Long: `Issue a positive goodwill account credit to a user.

This moves money into the user's balance and sends the backend credit
notification. Each successful call creates a new credit. If the request fails
after being sent, run "gumroad admin users credits list" for the user before
retrying to avoid issuing a duplicate credit.

Credits are capped at $1,000 per call unless --allow-large-amount is passed.
Negative credits and clawbacks are intentionally not supported by this CLI
command.`,
		Example: `  gumroad admin users credits add --user-id 2245593582708 --amount-cents 1000 --reason "Goodwill for checkout bug" --dry-run
  gumroad admin users credits add --user-id 2245593582708 --expected-email seller@example.com --amount-cents 1000 --reason "Goodwill for checkout bug" --yes
  gumroad admin users credits add --user-id 2245593582708 --amount-cents 150000 --reason "Approved large goodwill credit" --allow-large-amount --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}
			if err := validateCreditAmount(c, amountCents, allowLarge); err != nil {
				return err
			}
			trimmedReason, err := validateCreditReason(c, reason)
			if err != nil {
				return err
			}

			req := addCreditRequest{
				UserID:        target.UserID,
				AmountCents:   amountCents,
				Reason:        trimmedReason,
				ExpectedEmail: target.ExpectedEmail,
			}
			path := "users/add_credit"

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(
				"Issue %s credit to user_id %s? This adds funds to the user's balance.",
				formatCreditAmount(amountCents),
				target.UserID,
			))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "issue credit for user_id "+target.UserID, target.UserID)
			}

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), addCreditDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Issuing credit...", path, req)
			if err != nil {
				return wrapCreditError(target.UserID, err)
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[addCreditResponse](data)
			if err != nil {
				return err
			}
			return renderAddCredit(opts, fallback(decoded.UserID, target.UserID), decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().IntVar(&amountCents, "amount-cents", 0, "Credit amount in cents (required, positive)")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason shown in the creator notification and audit comment (required)")
	cmd.Flags().BoolVar(&allowLarge, "allow-large-amount", false, "Allow credits over the $1,000 per-call cap")

	return cmd
}

func newCreditsListCmd() *cobra.Command {
	var (
		lookup userLookupFlags
		page   cursor.Flags
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credits for a user",
		Long:  `List a user's account credits, newest first.`,
		Example: `  gumroad admin users credits list --user-id 2245593582708
  gumroad admin users credits list --email seller@example.com --limit 50
  gumroad admin users credits list --user-id 2245593582708 --cursor cur-next`,
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
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[creditsResponse](opts, "Fetching user credits...", "/users/credits", params, func(resp creditsResponse) error {
				return renderCreditsList(opts, target.Identifier(), resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)
	cursor.AddFlags(cmd, &page)

	return cmd
}

func validateCreditAmount(cmd *cobra.Command, amountCents int, allowLarge bool) error {
	if !cmd.Flags().Changed("amount-cents") {
		return cmdutil.MissingFlagError(cmd, "--amount-cents")
	}
	if amountCents <= 0 {
		return cmdutil.UsageErrorf(cmd, "--amount-cents must be greater than 0")
	}
	if amountCents > creditMaxCents && !allowLarge {
		return cmdutil.UsageErrorf(cmd, "--amount-cents exceeds the $1,000 per-call cap; pass --allow-large-amount to override")
	}
	return nil
}

func validateCreditReason(cmd *cobra.Command, reason string) (string, error) {
	if !cmd.Flags().Changed("reason") {
		return "", cmdutil.MissingFlagError(cmd, "--reason")
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "", cmdutil.UsageErrorf(cmd, "--reason cannot be empty")
	}
	return trimmed, nil
}

func addCreditDryRunParams(req addCreditRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	params.Set("amount_cents", strconv.Itoa(req.AmountCents))
	params.Set("reason", req.Reason)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	return params
}

func renderAddCredit(opts cmdutil.Options, userID string, resp addCreditResponse) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			strconv.FormatBool(resp.Success),
			userID,
			resp.Credit.ID,
			strconv.Itoa(resp.Credit.AmountCents),
			resp.Credit.Reason,
			resp.Credit.CreditingUserID,
			resp.Credit.CreatedAt,
		}})
	}

	if opts.Quiet {
		return nil
	}

	headline := "Credit issued"
	if err := output.Writeln(opts.Out(), opts.Style().Green(headline)); err != nil {
		return err
	}
	if err := cmdutil.WriteIdentifierLine(opts.Out(), "User ID", headline, userID); err != nil {
		return err
	}
	return writeCreditDetails(opts.Out(), resp.Credit)
}

func renderCreditsList(opts cmdutil.Options, identifier string, resp creditsResponse) error {
	if opts.PlainOutput {
		return writeCreditsPlain(opts.Out(), resp.Credits)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.Credits) == 0 {
			if err := output.Writef(w, "No credits found for %s.\n", identifier); err != nil {
				return err
			}
			return cursor.WriteMoreFooter(w, resp.Pagination)
		}

		if err := output.Writeln(w, style.Bold(fmt.Sprintf("%d credit(s) for %s", len(resp.Credits), identifier))); err != nil {
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
		if err := writeCreditsTable(w, style, resp.Credits); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writeCreditDetails(w io.Writer, credit creditPayload) error {
	if credit.ID != "" {
		if err := output.Writef(w, "Credit ID: %s\n", credit.ID); err != nil {
			return err
		}
	}
	if err := output.Writef(w, "Amount: %s\n", formatCreditAmount(credit.AmountCents)); err != nil {
		return err
	}
	if credit.Reason != "" {
		if err := output.Writef(w, "Reason: %s\n", credit.Reason); err != nil {
			return err
		}
	}
	if credit.CreditingUserID != "" {
		if err := output.Writef(w, "Crediting user: %s\n", credit.CreditingUserID); err != nil {
			return err
		}
	}
	if credit.CreatedAt != "" {
		return output.Writef(w, "Created: %s\n", credit.CreatedAt)
	}
	return nil
}

func writeCreditsPlain(w io.Writer, credits []creditPayload) error {
	rows := make([][]string, 0, len(credits))
	for _, credit := range credits {
		rows = append(rows, creditRow(credit))
	}
	return output.PrintPlain(w, rows)
}

func writeCreditsTable(w io.Writer, style output.Styler, credits []creditPayload) error {
	tbl := output.NewStyledTable(style, "ID", "AMOUNT", "REASON", "CREDITED BY", "CREATED")
	for _, credit := range credits {
		tbl.AddRow(
			credit.ID,
			formatCreditAmount(credit.AmountCents),
			credit.Reason,
			credit.CreditingUserID,
			credit.CreatedAt,
		)
	}
	return tbl.Render(w)
}

func creditRow(credit creditPayload) []string {
	return []string{
		credit.ID,
		strconv.Itoa(credit.AmountCents),
		credit.Reason,
		credit.CreditingUserID,
		credit.CreatedAt,
	}
}

func formatCreditAmount(cents int) string {
	amount := cmdutil.FormatMoney(cents, "")
	if strings.HasPrefix(amount, "-") {
		return fmt.Sprintf("-$%s (%d cents)", strings.TrimPrefix(amount, "-"), cents)
	}
	return fmt.Sprintf("$%s (%d cents)", amount, cents)
}

func wrapCreditError(userID string, err error) error {
	verify := fmt.Sprintf("Verify status with 'gumroad admin users credits list --user-id %s' before retrying to avoid duplicate credits", userID)

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    fmt.Sprintf("credit request failed: %s. %s", apiErr.Message, verify),
			Hint:       apiErr.Hint,
		}
	}
	return fmt.Errorf("credit request failed: %w. %s", err, verify)
}
