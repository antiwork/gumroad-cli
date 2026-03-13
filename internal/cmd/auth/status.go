package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	statusReasonNotLoggedIn      = "not_logged_in"
	statusReasonInvalidOrExpired = "invalid_or_expired"
	statusReasonAccessDenied     = "access_denied"
)

type statusOutput struct {
	Authenticated bool               `json:"authenticated"`
	User          json.RawMessage    `json:"user,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	Source        config.TokenSource `json:"source,omitempty"`
}

type authUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Show current authentication status",
		Args:    cmdutil.ExactArgs(0),
		Example: "  gumroad auth status",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			style := opts.Style()
			tokenInfo, err := config.ResolveToken()
			if err != nil {
				if !errors.Is(err, config.ErrNotAuthenticated) {
					return err
				}
				status := unauthenticatedStatus(statusReasonNotLoggedIn)
				if opts.UsesJSONOutput() {
					return printAuthJSON(opts, status)
				}
				if opts.PlainOutput {
					return printStatusPlain(opts, status)
				}
				return output.Writeln(opts.Out(), "Not logged in. Run "+style.Bold("gumroad auth login")+" or set "+style.Bold(config.EnvAccessToken)+" to authenticate.")
			}

			status, err := lookupStatus(opts, tokenInfo)
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return printAuthJSON(opts, status)
			}
			if opts.PlainOutput {
				return printStatusPlain(opts, status)
			}

			if !status.Authenticated {
				switch status.Reason {
				case statusReasonAccessDenied:
					return output.Writeln(opts.Out(), authSourceMessage(status.Source, "Token was accepted but access is denied. Check that it has the required scope.", "GUMROAD_ACCESS_TOKEN was accepted but access is denied. Check that it has the required scope."))
				default:
					return output.Writeln(opts.Out(), authSourceMessage(status.Source, "Token is invalid or expired. Run "+style.Bold("gumroad auth login")+" to re-authenticate.", "GUMROAD_ACCESS_TOKEN is invalid or expired. Update it in your shell and try again."))
				}
			}

			user, err := decodeAuthUser(status.User)
			if err != nil {
				return err
			}
			if err := writeAuthenticatedMessage(opts.Out(), style, user, "Authenticated."); err != nil {
				return err
			}
			return output.Writeln(opts.Out(), style.Dim("Source: "+authSourceLabel(status.Source)))
		},
	}
}

func lookupStatus(opts cmdutil.Options, tokenInfo config.TokenInfo) (statusOutput, error) {
	sp := output.NewSpinnerTo("Checking authentication...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, tokenInfo.Value)
	data, err := client.Get("/user", url.Values{})
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case 401:
				return unauthenticatedStatusWithSource(statusReasonInvalidOrExpired, tokenInfo.Source), nil
			case 403:
				return unauthenticatedStatusWithSource(statusReasonAccessDenied, tokenInfo.Source), nil
			}
		}
		return statusOutput{}, fmt.Errorf("could not verify token: %w", err)
	}

	resp, err := cmdutil.DecodeJSON[authUserEnvelope](data)
	if err != nil {
		return statusOutput{}, err
	}

	return statusOutput{
		Authenticated: true,
		User:          resp.User,
		Source:        tokenInfo.Source,
	}, nil
}

func unauthenticatedStatus(reason string) statusOutput {
	return statusOutput{
		Authenticated: false,
		Reason:        reason,
	}
}

func unauthenticatedStatusWithSource(reason string, source config.TokenSource) statusOutput {
	status := unauthenticatedStatus(reason)
	status.Source = source
	return status
}

func decodeAuthUser(data json.RawMessage) (authUser, error) {
	var user authUser
	if len(data) == 0 {
		return user, nil
	}
	if err := json.Unmarshal(data, &user); err != nil {
		return authUser{}, fmt.Errorf("could not parse response: %w", err)
	}
	return user, nil
}

func writeAuthenticatedMessage(w io.Writer, style output.Styler, user authUser, fallback string) error {
	switch {
	case user.Name != "" && user.Email != "":
		return output.Writeln(w, style.Green("✓")+" Logged in as "+style.Bold(user.Name)+" ("+user.Email+")")
	case user.Name != "":
		return output.Writeln(w, style.Green("✓")+" Logged in as "+style.Bold(user.Name))
	case user.Email != "":
		return output.Writeln(w, style.Green("✓")+" Logged in as "+style.Bold(user.Email))
	}
	return output.Writeln(w, style.Green("✓")+" "+fallback)
}

func printAuthJSON(opts cmdutil.Options, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("could not encode JSON output: %w", err)
	}
	return output.PrintJSON(opts.Out(), data, opts.JQExpr)
}

func printStatusPlain(opts cmdutil.Options, status statusOutput) error {
	user, err := decodeAuthUser(status.User)
	if err != nil {
		return err
	}

	return output.PrintPlain(opts.Out(), [][]string{
		{strconv.FormatBool(status.Authenticated), user.Name, user.Email, status.Reason},
	})
}

func authSourceLabel(source config.TokenSource) string {
	switch source {
	case config.TokenSourceEnv:
		return config.EnvAccessToken
	case config.TokenSourceConfig:
		return "stored config"
	default:
		return "unknown"
	}
}

func authSourceMessage(source config.TokenSource, configMessage, envMessage string) string {
	if source == config.TokenSourceEnv {
		return envMessage
	}
	return configMessage
}
