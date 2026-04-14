package auth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/oauth"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type authUserEnvelope struct {
	User json.RawMessage `json:"user"`
}

// isTerminalFunc checks whether the reader is a terminal. Replaceable in tests.
var isTerminalFunc = defaultIsTerminal

func newLoginCmd() *cobra.Command {
	var webFlag bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Gumroad",
		Args:  cmdutil.ExactArgs(0),
		Long: `Log in to Gumroad via browser-based OAuth or a manual API token.

By default, opens your browser for OAuth authorization.
When stdin is piped (e.g. echo $TOKEN | gumroad auth login), reads a token directly.`,
		Example: `  # Browser-based OAuth login (default)
  gumroad auth login

  # Explicit browser OAuth
  gumroad auth login --web

  # Pipe token from stdin (CI/scripts)
  echo "your-token" | gumroad auth login`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "store API token")
			}

			token, err := resolveToken(opts, webFlag)
			if err != nil {
				return err
			}
			if token == "" {
				return cmdutil.UsageErrorf(c, "token cannot be empty")
			}

			return verifyAndSave(c, opts, token)
		},
	}

	cmd.Flags().BoolVar(&webFlag, "web", false, "Force browser-based OAuth login")

	return cmd
}

func resolveToken(opts cmdutil.Options, webFlag bool) (string, error) {
	if !isTerminalFunc(opts.In()) {
		return prompt.TokenInput(opts.In(), opts.Err(), opts.NoInput)
	}
	return oauthLogin(opts, webFlag)
}

func defaultIsTerminal(r interface{}) bool {
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func oauthLogin(opts cmdutil.Options, webFlag bool) (string, error) {
	cfg := oauth.DefaultFlowConfig()

	token, browserErr := tryBrowserFlow(opts, cfg)
	if browserErr == nil {
		return token, nil
	}

	if webFlag {
		return "", fmt.Errorf("browser login failed: %w", browserErr)
	}

	// Fall back to headless flow.
	fmt.Fprintln(opts.Err(), "Could not open browser. Falling back to manual flow.")
	fmt.Fprintln(opts.Err())

	return oauth.HeadlessFlow(opts.Context, cfg, opts.Err(), stdinReader(opts.In()))
}

func tryBrowserFlow(opts cmdutil.Options, cfg oauth.FlowConfig) (string, error) {
	sp := output.NewSpinnerTo("Opening browser for authorization...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	return oauth.BrowserFlow(opts.Context, cfg, func(authURL string) error {
		sp.Stop()
		if err := oauth.OpenBrowser(authURL); err != nil {
			return err
		}
		fmt.Fprintln(opts.Err(), "Waiting for authorization in browser...")
		return nil
	})
}

func stdinReader(in io.Reader) func() (string, error) {
	return func() (string, error) {
		scanner := bufio.NewScanner(in)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("no input received")
		}
		return scanner.Text(), nil
	}
}

func verifyAndSave(c *cobra.Command, opts cmdutil.Options, token string) error {
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

	sp.Stop()

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
}
