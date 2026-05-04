package auth

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type logoutStatus struct {
	Authenticated  bool               `json:"authenticated"`
	LoggedOut      bool               `json:"logged_out"`
	AdminLoggedOut bool               `json:"admin_logged_out,omitempty"`
	Source         config.TokenSource `json:"source,omitempty"`
	Message        string             `json:"message,omitempty"`
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Log out of Gumroad",
		Args:    cmdutil.ExactArgs(0),
		Example: "  gumroad auth logout",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			confirmed, err := cmdutil.ConfirmAction(opts, "Remove stored API token?")
			if err != nil {
				return err
			}
			if !confirmed {
				return cmdutil.PrintCancelledAction(opts, "remove stored API token", "")
			}
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "remove stored API token")
			}

			adminLoggedOut, adminErr := revokeAndDeleteAdminToken(opts)
			if err := config.Delete(); err != nil {
				return err
			}
			if adminErr != nil {
				return adminErr
			}

			status := logoutStatus{
				Authenticated:  false,
				LoggedOut:      true,
				AdminLoggedOut: adminLoggedOut,
			}
			message := "Logged out successfully."
			tokenInfo, err := config.ResolveToken()
			if err == nil && tokenInfo.Source == config.TokenSourceEnv {
				status.Source = config.TokenSourceEnv
				message = config.EnvAccessToken + " is still set for this shell. Run `gumroad auth status` to verify it."
			}
			status.Message = message

			if opts.UsesJSONOutput() {
				return printAuthJSON(opts, status)
			}
			if opts.PlainOutput {
				row := []string{strconv.FormatBool(status.Authenticated), strconv.FormatBool(status.LoggedOut), string(status.Source)}
				if status.AdminLoggedOut {
					row = append(row, strconv.FormatBool(status.AdminLoggedOut))
				}
				return output.PrintPlain(opts.Out(), [][]string{row})
			}
			if opts.Quiet {
				return nil
			}
			style := opts.Style()
			if status.Source == config.TokenSourceEnv {
				return output.Writeln(opts.Out(), style.Green("✓")+" Removed stored token. "+style.Bold(config.EnvAccessToken)+" is still set for this shell. Run "+style.Bold("gumroad auth status")+" to verify it.")
			}
			return output.Writeln(opts.Out(), style.Green("✓")+" "+message)
		},
	}
}

func revokeAndDeleteAdminToken(opts cmdutil.Options) (bool, error) {
	tokenInfo, err := adminconfig.ResolveStoredToken()
	if err != nil {
		if errors.Is(err, adminconfig.ErrNotAuthenticated) {
			return false, nil
		}
		return false, err
	}

	client := adminapi.NewClientWithContext(opts.Context, tokenInfo.Value, opts.Version, opts.DebugEnabled())
	client.SetDebugWriter(opts.Err())
	if err := client.RevokeSelf(); err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 401 {
			return true, adminconfig.Delete()
		}
		return false, fmt.Errorf("couldn't revoke server-side; retry with 'gumroad auth logout', or revoke from %s", adminapi.AdminTokensURL())
	}
	return true, adminconfig.Delete()
}
