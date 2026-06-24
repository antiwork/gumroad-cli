package user

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const userPath = "/user"

type profile struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	Bio           string `json:"bio"`
	ProfileURL    string `json:"profile_url"`
	TwitterHandle string `json:"twitter_handle"`
}

type userResponse struct {
	User profile `json:"user"`
}

func NewUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Show and update account info",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad user
  gumroad user --json
  gumroad user --json --jq '.user.email'
  gumroad user update --name "Jane Doe" --bio "I make great things."
  gumroad user page publish ./landing.html`,
		RunE: func(c *cobra.Command, args []string) error {
			return runShow(cmdutil.OptionsFrom(c))
		},
	}

	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newPageCmd())

	return cmd
}

func runShow(opts cmdutil.Options) error {
	return cmdutil.RunRequestDecoded[userResponse](opts, "Fetching user info...", http.MethodGet, userPath, url.Values{}, func(resp userResponse) error {
		return renderUser(opts, resp.User, "")
	})
}

func renderUser(opts cmdutil.Options, u profile, header string) error {
	style := opts.Style()

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{u.Name, u.Email, u.ProfileURL},
		})
	}

	if header != "" {
		if opts.Quiet {
			return nil
		}
		if err := output.Writeln(opts.Out(), style.Green(header)); err != nil {
			return err
		}
	}

	if err := output.Writeln(opts.Out(), style.Bold(u.Name)); err != nil {
		return err
	}
	if err := output.Writeln(opts.Out(), u.Email); err != nil {
		return err
	}
	if u.Bio != "" {
		if err := output.Writeln(opts.Out(), style.Dim(u.Bio)); err != nil {
			return err
		}
	}
	if u.ProfileURL != "" {
		return output.Writeln(opts.Out(), u.ProfileURL)
	}

	return nil
}
