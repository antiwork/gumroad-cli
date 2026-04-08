package auth

import (
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type logoutStatus struct {
	Authenticated bool               `json:"authenticated"`
	LoggedOut     bool               `json:"logged_out"`
	Source        config.TokenSource `json:"source,omitempty"`
	Message       string             `json:"message,omitempty"`
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Remove stored authentication token",
		Args:    cmdutil.ExactArgs(0),
		Example: "  gumroad auth logout",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			confirmed, err := cmdutil.ConfirmAction(opts, "Remove stored API token?")
			if err != nil {
				return err
			}
			if !confirmed {
				return cmdutil.PrintCancelledAction(opts, "remove stored API token")
			}
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "remove stored API token")
			}

			if err := config.Delete(); err != nil {
				return err
			}

			status := logoutStatus{
				Authenticated: false,
				LoggedOut:     true,
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
				return output.PrintPlain(opts.Out(), [][]string{
					{strconv.FormatBool(status.Authenticated), strconv.FormatBool(status.LoggedOut), string(status.Source)},
				})
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
