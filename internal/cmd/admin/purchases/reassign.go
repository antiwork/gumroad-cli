package purchases

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type reassignRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type reassignResponse struct {
	Message               string      `json:"message"`
	Count                 api.JSONInt `json:"count"`
	ReassignedPurchaseIDs []string    `json:"reassigned_purchase_ids"`
}

func newReassignCmd() *cobra.Command {
	var (
		from string
		to   string
	)

	cmd := &cobra.Command{
		Use:   "reassign",
		Short: "Reassign every purchase from one buyer email to another",
		Long: `Reassign every purchase made by --from to --to. Subscriptions and the
underlying purchaser_id (when --to belongs to an existing user) are
updated as part of the move, and a grouped receipt is sent to --to.

This affects ALL successful purchases for --from. There is no way to
filter to a single purchase from this command.`,
		Example: `  gumroad admin purchases reassign --from old@example.com --to new@example.com
  gumroad admin purchases reassign --from old@example.com --to new@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if from == "" {
				return cmdutil.MissingFlagError(c, "--from")
			}
			if to == "" {
				return cmdutil.MissingFlagError(c, "--to")
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf("Reassign ALL purchases from %s to %s? A grouped receipt will be sent to %s.", from, to, to))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, fmt.Sprintf("reassign purchases from %s to %s", from, to), from)
			}

			req := reassignRequest{From: from, To: to}

			if opts.DryRun {
				params := url.Values{}
				params.Set("from", from)
				params.Set("to", to)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath("/purchases/reassign"), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Reassigning purchases...", "/purchases/reassign", req)
			if err != nil {
				return wrapReassignError(from, to, err)
			}

			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[reassignResponse](data)
			if err != nil {
				return err
			}
			return renderReassign(opts, from, to, decoded)
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Source buyer email (required)")
	cmd.Flags().StringVar(&to, "to", "", "Destination buyer email (required)")

	return cmd
}

func wrapReassignError(from, to string, err error) error {
	verifyHint := fmt.Sprintf(
		"Verify status with 'gumroad admin purchases search --email %s' (and --email %s) before retrying to avoid double-moves",
		from, to,
	)

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    fmt.Sprintf("reassign request failed: %s. %s", apiErr.Message, verifyHint),
			Hint:       apiErr.Hint,
		}
	}
	return fmt.Errorf("reassign request failed: %w. %s", err, verifyHint)
}

func renderReassign(opts cmdutil.Options, from, to string, resp reassignResponse) error {
	message := fallback(resp.Message, fmt.Sprintf("Reassigned purchases from %s to %s", from, to))
	count := fmt.Sprintf("%d", resp.Count)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, from, to, count}})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "From: %s\n", from); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "To: %s\n", to); err != nil {
		return err
	}
	if resp.Count > 0 {
		if err := output.Writef(opts.Out(), "Reassigned: %d purchase(s)\n", resp.Count); err != nil {
			return err
		}
	}
	if len(resp.ReassignedPurchaseIDs) > 0 {
		return output.Writef(opts.Out(), "Purchase IDs: %s\n", strings.Join(resp.ReassignedPurchaseIDs, ", "))
	}
	return nil
}
