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
	Email            string `json:"email"`
	RevenueThreshold string `json:"revenue_threshold"`
	Notes            string `json:"notes,omitempty"`
}

type updateWatchRequest struct {
	Email            string  `json:"email"`
	RevenueThreshold string  `json:"revenue_threshold"`
	Notes            *string `json:"notes,omitempty"`
}

type unwatchRequest struct {
	Email string `json:"email"`
}

type watchResponse struct {
	Success     bool             `json:"success"`
	Message     string           `json:"message"`
	WatchedUser *watchedUserInfo `json:"watched_user"`
}

type unwatchResponse struct {
	Success bool   `json:"success"`
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
		email            string
		revenueThreshold string
		note             string
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Add a user to the admin watchlist",
		Long: `Add a user to the admin watchlist without changing their risk state or
payout status. The revenue threshold is entered as a normal money amount, for
example 200 or 200.00.`,
		Example: `  gumroad admin users watch --email seller@example.com --revenue-threshold 200
  gumroad admin users watch --email seller@example.com --revenue-threshold 200 --note "Check next independent buyers"`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}
			threshold, thresholdCents, err := parseRevenueThreshold(c, revenueThreshold)
			if err != nil {
				return err
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf("Add %s to the watchlist with revenue threshold %s?", email, formatWatchMoney(thresholdCents)))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "watch user "+email, email)
			}

			req := watchRequest{Email: email, RevenueThreshold: threshold, Notes: note}
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
			return renderWatchAction(opts, email, decoded.Message, decoded.WatchedUser)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&revenueThreshold, "revenue-threshold", "", "Revenue threshold as a money amount, for example 200 or 200.00 (required)")
	cmd.Flags().StringVar(&note, "note", "", "Optional watch note")

	return cmd
}

func newUpdateWatchCmd() *cobra.Command {
	var (
		email            string
		revenueThreshold string
		note             string
		clearNote        bool
	)

	cmd := &cobra.Command{
		Use:   "update-watch",
		Short: "Update a user's active watchlist entry",
		Long: `Update the active watchlist entry for a user. When --note is omitted, the
existing note is preserved. Use --clear-note to remove the existing note.`,
		Example: `  gumroad admin users update-watch --email seller@example.com --revenue-threshold 500
  gumroad admin users update-watch --email seller@example.com --revenue-threshold 500 --note "Still monitoring"
  gumroad admin users update-watch --email seller@example.com --revenue-threshold 500 --clear-note`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}
			if c.Flags().Changed("note") && clearNote {
				return cmdutil.UsageErrorf(c, "--note and --clear-note cannot be used together")
			}
			threshold, thresholdCents, err := parseRevenueThreshold(c, revenueThreshold)
			if err != nil {
				return err
			}

			notes := watchNotesPointer(c, note, clearNote)
			confirmMsg := fmt.Sprintf("Update watch for %s to revenue threshold %s?", email, formatWatchMoney(thresholdCents))
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
				return cmdutil.PrintCancelledAction(opts, "update watch for "+email, email)
			}

			req := updateWatchRequest{Email: email, RevenueThreshold: threshold, Notes: notes}
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
			return renderWatchAction(opts, email, decoded.Message, decoded.WatchedUser)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&revenueThreshold, "revenue-threshold", "", "Revenue threshold as a money amount, for example 500 or 500.00 (required)")
	cmd.Flags().StringVar(&note, "note", "", "Watch note")
	cmd.Flags().BoolVar(&clearNote, "clear-note", false, "Clear the existing watch note")

	return cmd
}

func newUnwatchCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "unwatch",
		Short: "Remove a user from the admin watchlist",
		Example: `  gumroad admin users unwatch --email seller@example.com
  gumroad admin users unwatch --email seller@example.com --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			ok, err := cmdutil.ConfirmAction(opts, "Remove "+email+" from the watchlist?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "unwatch user "+email, email)
			}

			req := unwatchRequest{Email: email}
			path := "users/unwatch"

			if opts.DryRun {
				params := url.Values{}
				params.Set("email", email)
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
			return renderUnwatchAction(opts, email, decoded)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

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
	params.Set("email", req.Email)
	params.Set("revenue_threshold", req.RevenueThreshold)
	if req.Notes != "" {
		params.Set("notes", req.Notes)
	}
	return params
}

func updateWatchDryRunParams(req updateWatchRequest) url.Values {
	params := url.Values{}
	params.Set("email", req.Email)
	params.Set("revenue_threshold", req.RevenueThreshold)
	if req.Notes != nil {
		params.Set("notes", *req.Notes)
	}
	return params
}

func renderWatchAction(opts cmdutil.Options, email, message string, watchedUser *watchedUserInfo) error {
	message = fallback(message, "Watchlist updated")

	if opts.PlainOutput {
		row := []string{"true", message, email}
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
	if email != "" {
		if err := output.Writef(opts.Out(), "Email: %s\n", email); err != nil {
			return err
		}
	}
	return writeWatchedUser(opts.Out(), watchedUser)
}

func renderUnwatchAction(opts cmdutil.Options, email string, resp unwatchResponse) error {
	message := fallback(resp.Message, "User removed from watchlist")

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", message, email}})
	}
	if opts.Quiet {
		return nil
	}
	if err := output.Writeln(opts.Out(), opts.Style().Green(message)); err != nil {
		return err
	}
	if email != "" {
		return output.Writef(opts.Out(), "Email: %s\n", email)
	}
	return nil
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
