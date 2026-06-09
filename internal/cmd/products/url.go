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

func newURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "url <id>",
		Short:   "Print a product's share link",
		Args:    cmdutil.ExactArgs(1),
		Example: `  gumroad products url <id>`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[pageutil.ShowResponse](
				opts,
				"Fetching product URL...",
				http.MethodGet,
				cmdutil.JoinPath("products", args[0]),
				url.Values{},
				func(resp pageutil.ShowResponse) error {
					shareURL := pageutil.ShareURL(resp.Product)
					if shareURL == "" {
						return fmt.Errorf("product %s has no share link yet; run `gumroad products view %s` to confirm it exists", args[0], args[0])
					}
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{shareURL}})
					}
					return output.Writeln(opts.Out(), shareURL)
				},
			)
		},
	}
}
