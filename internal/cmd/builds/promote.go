package builds

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func itoa(n int) string { return strconv.Itoa(n) }

func newPromoteCmd() *cobra.Command {
	var product string
	c := &cobra.Command{
		Use:   "promote <build-id>",
		Short: "Make a deployed build the live one",
		Long:  "Make a deployed build the live one. The previous live build is superseded; promoting an older build rolls back.",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.UsageErrorf(c, "--product is required")
			}
			path := buildsPath(product, args[0], "promote")
			if opts.DryRun {
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, path, url.Values{})
			}
			return cmdutil.RunRequestWithSuccess(opts, "Promoting build...", http.MethodPost, path, url.Values{}, args[0],
				fmt.Sprintf("Build %s is live.", args[0]))
		},
	}
	c.Flags().StringVar(&product, "product", "", "Product ID or permalink (required)")
	return c
}
