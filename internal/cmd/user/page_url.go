package user

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url",
		Short: "Print your profile landing page URLs",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target := pageutil.ProfileTarget()
			return cmdutil.RunRequestDecoded[pageutil.ProfileShowResponse](
				opts,
				"Fetching page URL...",
				http.MethodGet,
				target.Path,
				url.Values{},
				func(resp pageutil.ProfileShowResponse) error {
					if resp.ProfileURL == "" {
						return fmt.Errorf("user response did not include profile_url")
					}
					embedURL := pageutil.ProfileEmbedURL(resp.ProfileURL)
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{resp.ProfileURL, embedURL}})
					}
					if err := output.Writeln(opts.Out(), resp.ProfileURL); err != nil {
						return err
					}
					return output.Writeln(opts.Out(), embedURL)
				},
			)
		},
	}
}
