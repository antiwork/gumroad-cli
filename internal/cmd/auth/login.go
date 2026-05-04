package auth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/adminconfig"
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

type loginCredentials struct {
	SellerToken string
	AdminToken  *adminconfig.Config
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
When stdin is piped (e.g. echo $TOKEN | gumroad auth login), reads a seller token directly.`,
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

			creds, err := resolveLoginCredentials(opts, webFlag)
			if err != nil {
				return err
			}
			if creds.SellerToken == "" {
				return cmdutil.UsageErrorf(c, "token cannot be empty")
			}

			return verifyAndSave(c, opts, creds)
		},
	}

	cmd.Flags().BoolVar(&webFlag, "web", false, "Force browser-based OAuth login")

	return cmd
}

func resolveLoginCredentials(opts cmdutil.Options, webFlag bool) (loginCredentials, error) {
	if !isTerminalFunc(opts.In()) {
		token, err := prompt.TokenInput(opts.In(), opts.Err(), opts.NoInput)
		return loginCredentials{SellerToken: token}, err
	}
	return oauthLogin(opts, webFlag)
}

func defaultIsTerminal(r interface{}) bool {
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func oauthLogin(opts cmdutil.Options, webFlag bool) (loginCredentials, error) {
	cfg := oauth.DefaultFlowConfig()

	result, browserErr := tryBrowserFlow(opts, cfg)
	if browserErr == nil {
		return loginCredentialsFromOAuthResult(opts, result)
	}

	if webFlag {
		return loginCredentials{}, fmt.Errorf("browser login failed: %w", browserErr)
	}

	// Fall back to headless flow.
	fmt.Fprintln(opts.Err(), "Could not open browser. Falling back to manual flow.")
	fmt.Fprintln(opts.Err())

	result, err := oauth.HeadlessFlowResult(opts.Context, cfg, opts.Err(), stdinReader(opts.In()))
	if err != nil {
		return loginCredentials{}, err
	}
	return loginCredentialsFromOAuthResult(opts, result)
}

func tryBrowserFlow(opts cmdutil.Options, cfg oauth.FlowConfig) (oauth.FlowResult, error) {
	sp := output.NewSpinnerTo("Opening browser for authorization...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	return oauth.BrowserFlowResult(opts.Context, cfg, func(authURL string) error {
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

func loginCredentialsFromOAuthResult(opts cmdutil.Options, result oauth.FlowResult) (loginCredentials, error) {
	creds := loginCredentials{SellerToken: result.AccessToken}
	switch {
	case result.AdminToken != nil:
		creds.AdminToken = adminConfigFromOAuth(result.AdminToken)
	case result.AdminAuthorizationCode != "":
		adminToken, err := adminapi.ExchangeAuthorizationCode(opts.Context, result.AdminAuthorizationCode, result.CodeVerifier, opts.Version, opts.DebugEnabled())
		if err != nil {
			return loginCredentials{}, fmt.Errorf("could not authorize admin token: %w", err)
		}
		creds.AdminToken = adminConfigFromExchange(adminToken)
	}
	return creds, nil
}

func adminConfigFromOAuth(token *oauth.AdminTokenResponse) *adminconfig.Config {
	if token == nil {
		return nil
	}
	return &adminconfig.Config{
		Token:           token.Token,
		TokenExternalID: token.TokenExternalID,
		Actor: adminconfig.Actor{
			Name:  token.Actor.Name,
			Email: token.Actor.Email,
		},
		ExpiresAt: token.ExpiresAt,
	}
}

func adminConfigFromExchange(token adminapi.AdminToken) *adminconfig.Config {
	return &adminconfig.Config{
		Token:           token.Token,
		TokenExternalID: token.TokenExternalID,
		Actor:           token.Actor,
		ExpiresAt:       token.ExpiresAt,
	}
}

func verifyAndSave(c *cobra.Command, opts cmdutil.Options, creds loginCredentials) error {
	sp := output.NewSpinnerTo("Verifying token...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, creds.SellerToken)
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

	if creds.AdminToken != nil {
		if creds.AdminToken.Token == "" {
			return fmt.Errorf("admin token response did not contain a token")
		}
		if err := revokeExistingAdminToken(opts); err != nil {
			return err
		}
	}

	if err := config.Save(&config.Config{AccessToken: creds.SellerToken}); err != nil {
		return fmt.Errorf("could not save token: %w", err)
	}
	if creds.AdminToken != nil {
		if err := adminconfig.Save(creds.AdminToken); err != nil {
			return fmt.Errorf("could not save admin token: %w", err)
		}
	}

	sp.Stop()

	if opts.UsesJSONOutput() {
		status := statusOutput{
			Authenticated: true,
			User:          resp.User,
		}
		if creds.AdminToken != nil {
			status.Admin = adminStatusFromConfig(creds.AdminToken, true, "")
		}
		return printAuthJSON(opts, status)
	}

	user, err := decodeAuthUser(resp.User)
	if err != nil {
		return err
	}

	if opts.PlainOutput {
		row := []string{"true", user.Name, user.Email}
		if creds.AdminToken != nil {
			row = append(row, "true", adminActorName(creds.AdminToken.Actor), creds.AdminToken.Actor.Email, creds.AdminToken.ExpiresAt)
		}
		return output.PrintPlain(opts.Out(), [][]string{row})
	}

	if opts.Quiet {
		return nil
	}

	if err := writeAuthenticatedMessage(opts.Out(), opts.Style(), user, "Logged in."); err != nil {
		return err
	}
	if creds.AdminToken != nil {
		return output.Writeln(opts.Out(), opts.Style().Green("✓")+" Admin operations authorized as "+opts.Style().Bold(adminActorName(creds.AdminToken.Actor)))
	}
	return nil
}

func revokeExistingAdminToken(opts cmdutil.Options) error {
	tokenInfo, err := adminconfig.ResolveStoredToken()
	if err != nil {
		if errors.Is(err, adminconfig.ErrNotAuthenticated) {
			return nil
		}
		return err
	}

	client := adminapi.NewClientWithContext(opts.Context, tokenInfo.Value, opts.Version, opts.DebugEnabled())
	client.SetDebugWriter(opts.Err())
	if err := client.RevokeSelf(); err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 401 {
			_ = adminconfig.Delete()
			return nil
		}
		return fmt.Errorf("could not revoke previous admin token: %w", err)
	}
	return nil
}
