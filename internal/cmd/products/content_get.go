package products

import (
	"encoding/json"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newContentGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <product_id>",
		Short: "Dump product rich content JSON",
		Long: "Dump a product's shared rich content page array as JSON.\n\n" +
			"The output is a JSON document intended for editing and passing back to `gumroad products content set`. Use `--jq` to filter it.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad products content get <product_id> > content.json
  gumroad products content get <product_id> --jq '.[].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if opts.PlainOutput {
				return cmdutil.UsageErrorf(c, "products content get outputs a rich content JSON document; omit --plain or use --jq to filter it")
			}

			requestOpts := opts
			if opts.UsesJSONOutput() {
				requestOpts.JSONOutput = false
				requestOpts.JQExpr = ""
			}

			productID := args[0]
			return cmdutil.Run(requestOpts, "Fetching content...", func(client *api.Client) (json.RawMessage, error) {
				return client.Get(cmdutil.JoinPath("products", productID), url.Values{})
			}, func(data json.RawMessage) error {
				resp, err := cmdutil.DecodeJSON[productContentResponse](data)
				if err != nil {
					return err
				}
				state := resp.state()
				if err := ensureSharedProductContent(productID, state); err != nil {
					return err
				}
				richContent, err := normalizeProductRichContent(state.RichContent)
				if err != nil {
					return err
				}
				return output.PrintJSON(opts.Out(), richContent, opts.JQExpr)
			})
		},
	}
}
