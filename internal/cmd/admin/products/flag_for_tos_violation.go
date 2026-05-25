package products

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmd/admin/users/usertarget"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const flagForTOSViolationConfirmationMessage = "Flag product %s for a policy violation on user_id %s? This notifies the seller and leaves the rest of the account online."

type flagForTOSViolationRequest struct {
	UserID        string `json:"user_id"`
	ProductID     string `json:"product_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

type flagForTOSViolationResponse struct {
	Success   bool   `json:"success"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

func newFlagForTOSViolationCmd() *cobra.Command {
	var targetFlags usertarget.MutationFlags

	cmd := &cobra.Command{
		Use:   "flag-for-tos-violation <product-id>",
		Short: "Flag a product for a policy violation as an admin",
		Long: `Flag a seller for a policy violation with a specific live product as context.

Identify the seller with --user-id. Pass --expected-email as an optional guard
against acting on an account whose email has changed.

This is a product-level action: the seller is notified and must resolve the
policy issue, but the account is not suspended and other products stay online.
The internal API records a standardized product-context comment for this action;
this command does not send a free-form note.`,
		Example: `  gumroad admin products flag-for-tos-violation abc123 --user-id 2245593582708
  gumroad admin products flag-for-tos-violation abc123 --user-id 2245593582708 --expected-email seller@example.com
  gumroad admin products flag-for-tos-violation abc123 --user-id 2245593582708 --yes`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID := args[0]

			target, err := usertarget.ResolveMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(flagForTOSViolationConfirmationMessage, productID, target.UserID))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "flag product "+productID+" for a policy violation on user_id "+target.UserID, productID)
			}

			req := flagForTOSViolationRequest{
				UserID:        target.UserID,
				ProductID:     productID,
				ExpectedEmail: target.ExpectedEmail,
			}
			path := "users/flag_for_tos_violation"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), flagForTOSViolationDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Flagging product for policy violation...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[flagForTOSViolationResponse](data)
			if err != nil {
				return err
			}
			userID := usertarget.Fallback(decoded.UserID, target.UserID)
			return renderFlagForTOSViolation(opts, userID, usertarget.Fallback(decoded.ProductID, productID), decoded)
		},
	}

	usertarget.AddMutationFlags(cmd, &targetFlags)

	return cmd
}

func flagForTOSViolationDryRunParams(req flagForTOSViolationRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	params.Set("product_id", req.ProductID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	return params
}

func renderFlagForTOSViolation(opts cmdutil.Options, userID, productID string, resp flagForTOSViolationResponse) error {
	message := usertarget.Fallback(resp.Message, resp.Status)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{"true", message, userID, productID, resp.Status},
		})
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
	if err := cmdutil.WriteIdentifierLine(opts.Out(), "Product ID", message, productID); err != nil {
		return err
	}
	if resp.Status != "" {
		return output.Writef(opts.Out(), "Status: %s\n", resp.Status)
	}
	return nil
}
