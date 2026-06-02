package products

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
		Use:   "url <product_id>",
		Short: "Print a product landing page URL",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target := pageutil.ProductTarget(args[0])
			return cmdutil.RunRequestDecoded[pageutil.ShowResponse](
				opts,
				"Fetching page URL...",
				http.MethodGet,
				target.Path,
				url.Values{},
				func(resp pageutil.ShowResponse) error {
					landingURL := pageutil.LandingURL(resp.Product)
					if landingURL == "" {
						return fmt.Errorf("product response did not include landing_url")
					}
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{landingURL}})
					}
					return output.Writeln(opts.Out(), landingURL)
				},
			)
		},
	}
}
