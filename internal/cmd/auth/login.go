package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/spf13/cobra"
)

type authUserEnvelope struct {
	User json.RawMessage `json:"user"`
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a Gumroad API token",
		Args:  cmdutil.ExactArgs(0),
		Long:  "Store your Gumroad API token for future use.\nThe token is read interactively (or from stdin when piped).",
		Example: `  # Interactive login
  gumroad auth login

  # Pipe token from stdin
  echo "your-token" | gumroad auth login`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "store API token")
			}
			token, err := prompt.TokenInput(opts.In(), opts.Err(), opts.NoInput)
			if err != nil {
				return err
			}
			if token == "" {
				return cmdutil.UsageErrorf(c, "token cannot be empty")
			}

			sp := output.NewSpinnerTo("Verifying token...", opts.Err())
			if cmdutil.ShouldShowSpinner(opts) {
				sp.Start()
			}
			defer sp.Stop()

			client := cmdutil.NewAPIClient(opts, token)
			data, err := client.Get("/user", url.Values{})
			if err != nil {
				var apiErr *api.APIError
				if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
					return fmt.Errorf("invalid token: %w", err)
				}
				return fmt.Errorf("could not verify token: %w", err)
			}

			resp, err := cmdutil.DecodeJSON[authUserEnvelope](data)
			if err != nil {
				return err
			}

			if err := config.Save(&config.Config{AccessToken: token}); err != nil {
				return fmt.Errorf("could not save token: %w", err)
			}

			if opts.UsesJSONOutput() {
				return printAuthJSON(opts, statusOutput{
					Authenticated: true,
					User:          resp.User,
				})
			}

			user, err := decodeAuthUser(resp.User)
			if err != nil {
				return err
			}

			if opts.PlainOutput {
				return output.PrintPlain(opts.Out(), [][]string{
					{"true", user.Name, user.Email},
				})
			}

			if opts.Quiet {
				return nil
			}

			return writeAuthenticatedMessage(opts.Out(), opts.Style(), user, "Logged in.")
		},
	}
}
