package products

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmd/admin/users/usertarget"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	holdSalesConfirmationMessage        = "Hold new sales of product %s on user_id %s? This unpublishes the listing if it is live and blocks new checkouts until the hold is released. The seller is not emailed and the rest of the account is unchanged."
	releaseSalesHoldConfirmationMessage = "Release sales hold %s on product %s for user_id %s? This does not republish the listing."

	salesHoldStatusHeld            = "held"
	salesHoldStatusAlreadyHeld     = "already_held"
	salesHoldStatusReleased        = "released"
	salesHoldStatusAlreadyReleased = "already_released"
)

type holdSalesRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

type releaseSalesHoldRequest struct {
	UserID        string `json:"user_id"`
	HoldID        string `json:"hold_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

type salesHold struct {
	ID                 string `json:"id"`
	Active             bool   `json:"active"`
	UnpublishedListing bool   `json:"unpublished_listing"`
	HeldAt             string `json:"held_at"`
	HeldByID           string `json:"held_by_id"`
	ReleasedAt         string `json:"released_at"`
	ReleasedByID       string `json:"released_by_id"`
}

type salesHoldProduct struct {
	ID                 string `json:"id"`
	Alive              bool   `json:"alive"`
	Published          bool   `json:"published"`
	Draft              bool   `json:"draft"`
	PurchaseDisabledAt string `json:"purchase_disabled_at"`
	DeletedAt          string `json:"deleted_at"`
	BannedAt           string `json:"banned_at"`
	UnpublishedByAdmin bool   `json:"unpublished_by_admin"`
	ActiveSalesHoldID  string `json:"active_sales_hold_id"`
}

type salesHoldResponse struct {
	Status    string           `json:"status"`
	SalesHold *salesHold       `json:"sales_hold"`
	Product   salesHoldProduct `json:"product"`
}

type salesHoldTarget struct {
	ProductID     string
	UserID        string
	ExpectedEmail string
}

func newHoldSalesCmd() *cobra.Command {
	var targetFlags usertarget.MutationFlags

	cmd := &cobra.Command{
		Use:   "hold-sales <product-id>",
		Short: "Hold new sales of one product as an admin",
		Long: `Hold new sales of one product without suspending its seller.

Identify the seller with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed. The server refuses the
hold if the product does not belong to that seller.

The hold unpublishes the listing if it is live, blocks new checkouts (including
bundles that contain it) and stops the seller from publishing it again. The
account, its other products, files, content and existing buyers' access are left
alone, and the seller is not emailed. Charges already under way, recurring and
preorder charges are not cancelled.

If the product already has an active hold, the server returns that hold with
status already_held instead of creating another. Keep the returned hold ID: it
is what release-sales-hold needs. To read it later, use
'gumroad admin products view <product-id> --json' and .product.active_sales_hold.

Requires a per-actor admin token and a Gumroad API that includes product sales
holds.`,
		Example: `  gumroad admin products hold-sales abc123 --user-id 2245593582708
  gumroad admin products hold-sales abc123 --user-id 2245593582708 --expected-email seller@example.com
  gumroad admin products hold-sales abc123 --user-id 2245593582708 --dry-run
  gumroad admin products hold-sales abc123 --user-id 2245593582708 --yes --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveSalesHoldTarget(c, args[0], targetFlags)
			if err != nil {
				return err
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(holdSalesConfirmationMessage, target.ProductID, target.UserID))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "hold sales of product "+target.ProductID+" on user_id "+target.UserID, target.ProductID)
			}

			req := holdSalesRequest{UserID: target.UserID, ExpectedEmail: target.ExpectedEmail}
			path := cmdutil.JoinPath("products", target.ProductID, "hold_sales")

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), salesHoldDryRunParams(target, ""))
			}

			data, err := postSalesHold(opts, "Holding product sales...", "hold-sales", target.ProductID, path, req)
			if err != nil {
				return err
			}
			return renderSalesHoldResult(opts, target.ProductID, data)
		},
	}

	usertarget.AddMutationFlags(cmd, &targetFlags)

	return cmd
}

func newReleaseSalesHoldCmd() *cobra.Command {
	var (
		targetFlags usertarget.MutationFlags
		holdID      string
	)

	cmd := &cobra.Command{
		Use:   "release-sales-hold <product-id>",
		Short: "Release one sales hold on a product as an admin",
		Long: `Release one sales hold on a product, identified by its hold ID.

Identify the seller with --user-id and the hold with --hold-id. Pass
--expected-email as an optional guard against acting on an account whose email
has changed. The hold ID comes from hold-sales, or from
'gumroad admin products view <product-id> --json' at .product.active_sales_hold.id.

Only the named hold is released. If that hold was replaced by a newer one, the
newer hold stays active and the product remains held; the output shows the
product's active hold after the release. This command never releases a hold it
was not given.

Releasing does not republish the listing. If the hold unpublished it, it stays
unpublished until the seller publishes it again. Bans, deletion and other admin
restrictions are not affected.

Requires a per-actor admin token and a Gumroad API that includes product sales
holds.`,
		Example: `  gumroad admin products release-sales-hold abc123 --user-id 2245593582708 --hold-id hold_abc
  gumroad admin products release-sales-hold abc123 --user-id 2245593582708 --hold-id hold_abc --expected-email seller@example.com
  gumroad admin products release-sales-hold abc123 --user-id 2245593582708 --hold-id hold_abc --dry-run
  gumroad admin products release-sales-hold abc123 --user-id 2245593582708 --hold-id hold_abc --yes --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			target, err := resolveSalesHoldTarget(c, args[0], targetFlags)
			if err != nil {
				return err
			}
			trimmedHoldID := strings.TrimSpace(holdID)
			if trimmedHoldID == "" {
				return cmdutil.MissingFlagError(c, "--hold-id")
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(releaseSalesHoldConfirmationMessage, trimmedHoldID, target.ProductID, target.UserID))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "release sales hold "+trimmedHoldID+" on product "+target.ProductID, target.ProductID)
			}

			req := releaseSalesHoldRequest{UserID: target.UserID, HoldID: trimmedHoldID, ExpectedEmail: target.ExpectedEmail}
			path := cmdutil.JoinPath("products", target.ProductID, "release_sales_hold")

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), salesHoldDryRunParams(target, trimmedHoldID))
			}

			data, err := postSalesHold(opts, "Releasing sales hold...", "release-sales-hold", target.ProductID, path, req)
			if err != nil {
				return err
			}
			return renderSalesHoldResult(opts, target.ProductID, data)
		},
	}

	usertarget.AddMutationFlags(cmd, &targetFlags)
	cmd.Flags().StringVar(&holdID, "hold-id", "", "ID of the sales hold to release (required)")

	return cmd
}

func resolveSalesHoldTarget(c *cobra.Command, productArg string, flags usertarget.MutationFlags) (salesHoldTarget, error) {
	productID := strings.TrimSpace(productArg)
	if productID == "" {
		return salesHoldTarget{}, cmdutil.UsageErrorf(c, "product ID must not be blank")
	}

	target, err := usertarget.ResolveMutationTarget(c, flags)
	if err != nil {
		return salesHoldTarget{}, err
	}
	userID := strings.TrimSpace(target.UserID)
	if userID == "" {
		return salesHoldTarget{}, cmdutil.MissingFlagError(c, "--user-id")
	}

	expectedEmail := strings.TrimSpace(target.ExpectedEmail)
	if expectedEmail == "" && (c.Flags().Changed("expected-email") || c.Flags().Changed("email")) {
		return salesHoldTarget{}, cmdutil.UsageErrorf(c, "--expected-email must not be blank; omit it to skip the email guard")
	}

	return salesHoldTarget{ProductID: productID, UserID: userID, ExpectedEmail: expectedEmail}, nil
}

func salesHoldDryRunParams(target salesHoldTarget, holdID string) url.Values {
	params := usertarget.MutationParams(usertarget.MutationTarget{UserID: target.UserID, ExpectedEmail: target.ExpectedEmail})
	if holdID != "" {
		params.Set("hold_id", holdID)
	}
	return params
}

func postSalesHold(opts cmdutil.Options, spinnerMessage, command, productID, path string, payload any) (json.RawMessage, error) {
	return admincmd.FetchMutation(opts, spinnerMessage, func(client *adminapi.Client) (json.RawMessage, error) {
		data, err := client.PostJSON(path, payload)
		if err != nil {
			return nil, wrapUncertainSalesHoldError(command, productID, err)
		}
		return data, nil
	})
}

// The server documents every refusal (bad input, unknown product or hold,
// wrong seller, changed owner) as writing nothing, so those errors pass through
// unchanged. A lost connection, a timeout, a rate limit or a server error leaves
// it unknown whether the hold changed, so the operator is told to check the
// product before running the command again. The request is never resent here.
func wrapUncertainSalesHoldError(command, productID string, err error) error {
	verify := fmt.Sprintf("The %s result is unknown and the request was not resent. Check .product.active_sales_hold with 'gumroad admin products view %s --json' before running it again", command, productID)

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s request failed: %w. %s", command, err, verify)
	}
	if !isUncertainSalesHoldStatus(apiErr.StatusCode) {
		return err
	}
	return &api.APIError{
		StatusCode: apiErr.StatusCode,
		Message:    fmt.Sprintf("%s request failed: %s. %s", command, apiErr.Message, verify),
	}
}

func isUncertainSalesHoldStatus(status int) bool {
	return status >= http.StatusInternalServerError ||
		status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests
}

func renderSalesHoldResult(opts cmdutil.Options, productID string, data json.RawMessage) error {
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}

	resp, err := cmdutil.DecodeJSON[salesHoldResponse](data)
	if err != nil {
		return err
	}
	if resp.SalesHold == nil || resp.SalesHold.ID == "" {
		return fmt.Errorf("server answered %q without a sales hold ID; check .product.active_sales_hold with 'gumroad admin products view %s --json'", resp.Status, productID)
	}
	resp.Product.ID = usertarget.Fallback(resp.Product.ID, productID)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			resp.Status,
			resp.Product.ID,
			resp.SalesHold.ID,
			strconv.FormatBool(resp.SalesHold.Active),
			resp.Product.ActiveSalesHoldID,
			strconv.FormatBool(resp.Product.Published),
		}})
	}

	remaining := remainingSalesHoldWarning(resp)
	if opts.Quiet {
		if remaining == "" {
			return nil
		}
		return output.Writeln(opts.Err(), opts.Style().Yellow(remaining))
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(salesHoldHeadline(resp))); err != nil {
		return err
	}
	lines := []string{
		"Status: " + resp.Status,
		"Hold ID: " + resp.SalesHold.ID,
		"Hold active: " + yesNo(resp.SalesHold.Active),
	}
	if resp.SalesHold.HeldAt != "" {
		lines = append(lines, "Held at: "+resp.SalesHold.HeldAt)
	}
	if resp.SalesHold.ReleasedAt != "" {
		lines = append(lines, "Released at: "+resp.SalesHold.ReleasedAt)
	}
	lines = append(lines,
		"Unpublished by this hold: "+yesNo(resp.SalesHold.UnpublishedListing),
		"Product active hold: "+usertarget.Fallback(resp.Product.ActiveSalesHoldID, "none"),
		"Product published: "+yesNo(resp.Product.Published),
	)
	for _, line := range lines {
		if err := output.Writeln(opts.Out(), line); err != nil {
			return err
		}
	}

	if remaining != "" {
		return output.Writeln(opts.Out(), style.Yellow(remaining))
	}
	if !resp.SalesHold.Active && !resp.Product.Published {
		return output.Writeln(opts.Out(), style.Dim("Releasing does not republish the listing. Bans, deletion and other admin restrictions are unchanged."))
	}
	return nil
}

func salesHoldHeadline(resp salesHoldResponse) string {
	switch resp.Status {
	case salesHoldStatusHeld:
		return "Sales held on product " + resp.Product.ID
	case salesHoldStatusAlreadyHeld:
		return "Product " + resp.Product.ID + " already had an active sales hold"
	case salesHoldStatusReleased:
		return "Released sales hold " + resp.SalesHold.ID + " on product " + resp.Product.ID
	case salesHoldStatusAlreadyReleased:
		return "Sales hold " + resp.SalesHold.ID + " on product " + resp.Product.ID + " was already released"
	default:
		return "Sales hold " + resp.SalesHold.ID + " on product " + resp.Product.ID
	}
}

func remainingSalesHoldWarning(resp salesHoldResponse) string {
	if resp.SalesHold.Active || resp.Product.ActiveSalesHoldID == "" || resp.Product.ActiveSalesHoldID == resp.SalesHold.ID {
		return ""
	}
	return "Product " + resp.Product.ID + " is still held by a different active sales hold " + resp.Product.ActiveSalesHoldID + "; new sales stay blocked."
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
