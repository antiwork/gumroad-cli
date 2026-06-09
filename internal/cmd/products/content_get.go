package products

import (
	"encoding/json"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newContentGetCmd() *cobra.Command {
	var variantID, categoryID string

	cmd := &cobra.Command{
		Use:   "get <product_id>",
		Short: "Dump product rich content JSON",
		Long: "Dump a product's rich content page array as JSON.\n\n" +
			"The output is a JSON document intended for editing and passing back to `gumroad products content set`. Use `--jq` to filter it.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad products content get <product_id> > content.json
  gumroad products content get <product_id> --variant <variant_id> --category <cat_id> > content.json
  gumroad products content get <product_id> --jq '.[].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if opts.PlainOutput {
				return cmdutil.UsageErrorf(c, "products content get outputs a rich content JSON document; omit --plain or use --jq to filter it")
			}
			if err := validateProductContentVariantFlags(c, variantID, categoryID); err != nil {
				return err
			}

			requestOpts := opts
			if opts.UsesJSONOutput() {
				requestOpts.JSONOutput = false
				requestOpts.JQExpr = ""
			}

			productID := args[0]
			return cmdutil.Run(requestOpts, "Fetching content...", func(client *api.Client) (json.RawMessage, error) {
				state, err := fetchProductContentState(client, productID)
				if err != nil {
					return nil, err
				}
				target, err := resolveProductContentTarget(productID, state, variantID, categoryID)
				if err != nil {
					return nil, err
				}
				if target.usesVariant() {
					variantState, err := fetchVariantContentState(client, target.Path)
					if err != nil {
						return nil, err
					}
					return normalizeProductRichContent(variantState.RichContent)
				}
				return normalizeProductRichContent(state.RichContent)
			}, func(data json.RawMessage) error {
				return output.PrintJSON(opts.Out(), data, opts.JQExpr)
			})
		},
	}

	cmd.Flags().StringVar(&variantID, "variant", "", "Variant ID for per-variant content")
	cmd.Flags().StringVar(&categoryID, "category", "", "Variant category ID for per-variant content")

	return cmd
}
