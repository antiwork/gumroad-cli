package user

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

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
	return &cobra.Command{
		Use:   "user",
		Short: "Show account info",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad user
  gumroad user --json
  gumroad user --json --jq '.user.email'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[userResponse](opts, "Fetching user info...", "GET", "/user", url.Values{}, func(resp userResponse) error {
				u := resp.User
				style := opts.Style()

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{u.Name, u.Email, u.ProfileURL},
					})
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
			})
		},
	}
}
