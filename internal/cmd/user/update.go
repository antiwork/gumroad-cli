package user

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var name, bio string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update your seller name and bio",
		Args:  cmdutil.ExactArgs(0),
		Long: "Update the seller name and bio shown on your profile.\n\n" +
			"Pass --name and/or --bio. Pass an empty value to clear a field.",
		Example: `  gumroad user update --name "Jane Doe"
  gumroad user update --bio "I make great things."
  gumroad user update --name "Jane Doe" --bio "I make great things."`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireAnyFlagChanged(c, "name", "bio"); err != nil {
				return err
			}

			params := url.Values{}
			if c.Flags().Changed("name") {
				params.Set("name", name)
			}
			if c.Flags().Changed("bio") {
				params.Set("bio", bio)
			}

			return cmdutil.RunRequestDecoded[userResponse](opts,
				"Updating user...", http.MethodPut, userPath, params,
				func(resp userResponse) error {
					return renderUser(opts, resp.User, "Updated user.")
				})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Seller name; pass an empty value to clear it")
	cmd.Flags().StringVar(&bio, "bio", "", "Seller bio; pass an empty value to clear it")

	return cmd
}
