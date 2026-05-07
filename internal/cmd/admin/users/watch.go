package users

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type watchRequest struct {
	UserID           string `json:"user_id"`
	ExpectedEmail    string `json:"expected_email,omitempty"`
	RevenueThreshold string `json:"revenue_threshold"`
	Notes            string `json:"notes,omitempty"`
}

type updateWatchRequest struct {
	UserID           string  `json:"user_id"`
	ExpectedEmail    string  `json:"expected_email,omitempty"`
	RevenueThreshold string  `json:"revenue_threshold"`
	Notes            *string `json:"notes,omitempty"`
}

type unwatchRequest struct {
	UserID        string `json:"user_id"`
	ExpectedEmail string `json:"expected_email,omitempty"`
}

type watchResponse struct {
	Success     bool             `json:"success"`
	UserID      string           `json:"user_id"`
	Message     string           `json:"message"`
	WatchedUser *watchedUserInfo `json:"watched_user"`
}

type unwatchResponse struct {
	Success bool   `json:"success"`
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

type watchedUserInfo struct {
	ID                    string `json:"id"`
	RevenueThresholdCents int    `json:"revenue_threshold_cents"`
	RevenueCents          int    `json:"revenue_cents"`
	UnpaidBalanceCents    int    `json:"unpaid_balance_cents"`
	Notes                 string `json:"notes"`
	CreatedAt             string `json:"created_at"`
	LastSyncedAt          string `json:"last_synced_at"`
}

func newWatchCmd() *cobra.Command {
	var (
		targetFlags      userMutationFlags
		revenueThreshold string
		note             string
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Add a user to the admin watchlist",
		Long: `Add a user to the admin watchlist without changing their risk state or
payout status. The revenue threshold is entered as a normal money amount, for
example 200 or 200.00.`,
		Example: `  gumroad admin users watch --user-id 2245593582708 --revenue-threshold 200
  gumroad admin users watch --user-id 2245593582708 --expected-email seller@example.com --revenue-threshold 200 --note "Check next independent buyers"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}
			threshold, thresholdCents, err := parseRevenueThreshold(c, revenueThreshold)
			if err != nil {
				return err
			}

			identifier := target.identifier()
			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf("Add user_id %s to the watchlist with revenue threshold %s?", identifier, formatWatchMoney(thresholdCents)))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "watch user_id "+identifier, identifier)
			}

			req := watchRequest{UserID: target.UserID, ExpectedEmail: target.ExpectedEmail, RevenueThreshold: threshold, Notes: note}
			path := "users/watch"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), watchDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Adding user to watchlist...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[watchResponse](data)
			if err != nil {
				return err
			}
			return renderWatchAction(opts, fallback(decoded.UserID, identifier), decoded.Message, decoded.WatchedUser)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().StringVar(&revenueThreshold, "revenue-threshold", "", "Revenue threshold as a money amount, for example 200 or 200.00 (required)")
	cmd.Flags().StringVar(&note, "note", "", "Optional watch note")

	return cmd
}

func newUpdateWatchCmd() *cobra.Command {
	var (
		targetFlags      userMutationFlags
		revenueThreshold string
		note             string
		clearNote        bool
	)

	cmd := &cobra.Command{
		Use:   "update-watch",
		Short: "Update a user's active watchlist entry",
		Long: `Update the active watchlist entry for a user. When --note is omitted, the
existing note is preserved. Use --clear-note to remove the existing note.`,
		Example: `  gumroad admin users update-watch --user-id 2245593582708 --revenue-threshold 500
  gumroad admin users update-watch --user-id 2245593582708 --revenue-threshold 500 --note "Still monitoring"
  gumroad admin users update-watch --user-id 2245593582708 --revenue-threshold 500 --clear-note`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}
			if c.Flags().Changed("note") && clearNote {
				return cmdutil.UsageErrorf(c, "--note and --clear-note cannot be used together")
			}
			threshold, thresholdCents, err := parseRevenueThreshold(c, revenueThreshold)
			if err != nil {
				return err
			}

			notes := watchNotesPointer(c, note, clearNote)
			identifier := target.identifier()
			confirmMsg := fmt.Sprintf("Update watch for user_id %s to revenue threshold %s?", identifier, formatWatchMoney(thresholdCents))
			switch {
			case clearNote:
				confirmMsg += " (note will be cleared)"
			case c.Flags().Changed("note"):
				confirmMsg += " (note will be updated)"
			}

			ok, err := cmdutil.ConfirmAction(opts, confirmMsg)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "update watch for user_id "+identifier, identifier)
			}

			req := updateWatchRequest{UserID: target.UserID, ExpectedEmail: target.ExpectedEmail, RevenueThreshold: threshold, Notes: notes}
			path := "users/update_watch"

			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), updateWatchDryRunParams(req))
			}

			data, err := admincmd.FetchPostJSON(opts, "Updating watchlist entry...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[watchResponse](data)
			if err != nil {
				return err
			}
			return renderWatchAction(opts, fallback(decoded.UserID, identifier), decoded.Message, decoded.WatchedUser)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)
	cmd.Flags().StringVar(&revenueThreshold, "revenue-threshold", "", "Revenue threshold as a money amount, for example 500 or 500.00 (required)")
	cmd.Flags().StringVar(&note, "note", "", "Watch note")
	cmd.Flags().BoolVar(&clearNote, "clear-note", false, "Clear the existing watch note")

	return cmd
}

func newUnwatchCmd() *cobra.Command {
	var targetFlags userMutationFlags

	cmd := &cobra.Command{
		Use:   "unwatch",
		Short: "Remove a user from the admin watchlist",
		Example: `  gumroad admin users unwatch --user-id 2245593582708
  gumroad admin users unwatch --user-id 2245593582708 --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserMutationTarget(c, targetFlags)
			if err != nil {
				return err
			}

			identifier := target.identifier()
			ok, err := cmdutil.ConfirmAction(opts, "Remove user_id "+identifier+" from the watchlist?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "unwatch user_id "+identifier, identifier)
			}

			req := unwatchRequest(target)
			path := "users/unwatch"

			if opts.DryRun {
				params := userMutationParams(target)
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), params)
			}

			data, err := admincmd.FetchPostJSON(opts, "Removing user from watchlist...", path, req)
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			decoded, err := cmdutil.DecodeJSON[unwatchResponse](data)
			if err != nil {
				return err
			}
			return renderUnwatchAction(opts, fallback(decoded.UserID, identifier), decoded)
		},
	}

	addUserMutationFlags(cmd, &targetFlags)

	return cmd
}

func parseRevenueThreshold(cmd *cobra.Command, value string) (string, int, error) {
	if value == "" {
		return "", 0, cmdutil.MissingFlagError(cmd, "--revenue-threshold")
	}
	cents, err := cmdutil.ParseMoney("revenue-threshold", value, "revenue threshold", "")
	if err != nil {
		return "", 0, cmdutil.UsageErrorf(cmd, "%s", err.Error())
	}
	if cents <= 0 {
		return "", 0, cmdutil.UsageErrorf(cmd, "--revenue-threshold must be greater than 0")
	}
	return cmdutil.FormatMoney(cents, ""), cents, nil
}

func watchNotesPointer(cmd *cobra.Command, note string, clearNote bool) *string {
	if clearNote {
		empty := ""
		return &empty
	}
	if cmd.Flags().Changed("note") {
		return &note
	}
	return nil
}

func watchDryRunParams(req watchRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	params.Set("revenue_threshold", req.RevenueThreshold)
	if req.Notes != "" {
		params.Set("notes", req.Notes)
	}
	return params
}

func updateWatchDryRunParams(req updateWatchRequest) url.Values {
	params := url.Values{}
	params.Set("user_id", req.UserID)
	if req.ExpectedEmail != "" {
		params.Set("expected_email", req.ExpectedEmail)
	}
	params.Set("revenue_threshold", req.RevenueThreshold)
	if req.Notes != nil {
		params.Set("notes", *req.Notes)
	}
	return params
}

func renderWatchAction(opts cmdutil.Options, userID, message string, watchedUser *watchedUserInfo) error {
	message = fallback(message, "Watchlist updated")

	if opts.PlainOutput {
		row := []string{"true", message, userID}
		if watchedUser != nil {
			row = append(row,
				watchedUser.ID,
				strconv.Itoa(watchedUser.RevenueThresholdCents),
				strconv.Itoa(watchedUser.RevenueCents),
				strconv.Itoa(watchedUser.UnpaidBalanceCents),
				watchedUser.Notes,
				watchedUser.CreatedAt,
				watchedUser.LastSyncedAt,
			)
		}
		return output.PrintPlain(opts.Out(), [][]string{row})
	}

	if opts.Quiet {
		return nil
	}

	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	if err := writeIdentifierLine(opts.Out(), "User ID", message, userID); err != nil {
		return err
	}
	return writeWatchedUser(opts.Out(), watchedUser)
}

func renderUnwatchAction(opts cmdutil.Options, userID string, resp unwatchResponse) error {
	message := fallback(resp.Message, "User removed from watchlist")

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, userID}})
	}
	if opts.Quiet {
		return nil
	}
	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	return writeIdentifierLine(opts.Out(), "User ID", message, userID)
}

func writeWatchedUser(w io.Writer, watchedUser *watchedUserInfo) error {
	if watchedUser == nil {
		return nil
	}
	if watchedUser.ID != "" {
		if err := output.Writef(w, "Watch ID: %s\n", watchedUser.ID); err != nil {
			return err
		}
	}
	if watchedUser.RevenueThresholdCents > 0 {
		if err := output.Writef(w, "Revenue: %s of %s\n", formatWatchMoney(watchedUser.RevenueCents), formatWatchMoney(watchedUser.RevenueThresholdCents)); err != nil {
			return err
		}
	}
	if watchedUser.UnpaidBalanceCents > 0 {
		if err := output.Writef(w, "Unpaid balance: %s\n", formatWatchMoney(watchedUser.UnpaidBalanceCents)); err != nil {
			return err
		}
	}
	if watchedUser.Notes != "" {
		if err := output.Writef(w, "Note: %s\n", watchedUser.Notes); err != nil {
			return err
		}
	}
	if watchedUser.CreatedAt != "" {
		if err := output.Writef(w, "Created: %s\n", watchedUser.CreatedAt); err != nil {
			return err
		}
	}
	if watchedUser.LastSyncedAt != "" {
		return output.Writef(w, "Last synced: %s\n", watchedUser.LastSyncedAt)
	}
	return nil
}

func formatWatchMoney(cents int) string {
	return "$" + cmdutil.FormatMoney(cents, "")
}
