package products

import (
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <id>",
		Short: "Publish a product",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			message := "Product " + args[0] + " published."
			return cmdutil.RunRequestDecoded[pageutil.ShowResponse](
				opts,
				"Publishing product...",
				http.MethodPut,
				cmdutil.JoinPath("products", args[0], "enable"),
				url.Values{},
				func(resp pageutil.ShowResponse) error {
					shareURL := pageutil.ShareURL(resp.Product)
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{"true", message, shareURL}})
					}
					if err := cmdutil.PrintSuccess(opts, message); err != nil {
						return err
					}
					if shareURL != "" && !opts.Quiet {
						return output.Writef(opts.Out(), "URL: %s\n", shareURL)
					}
					return nil
				},
			)
		},
	}
}
