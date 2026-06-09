package products

import (
	"encoding/json"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newContentGetCmd() *cobra.Command {
	var variantID, categoryID, pageID string

	cmd := &cobra.Command{
		Use:   "get <product_id>",
		Short: "Dump product rich content JSON",
		Long: "Dump a product's rich content page array as JSON.\n\n" +
			"The output is a JSON document intended for editing and passing back to `gumroad products content set`. Pass `--page` to dump one page object. Use `--jq` to filter it.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad products content get <product_id> > content.json
  gumroad products content get <product_id> --variant <variant_id> --category <cat_id> > content.json
  gumroad products content get <product_id> --page <page_id> > page.json
  gumroad products content get <product_id> --jq '.[].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if opts.PlainOutput {
				return cmdutil.UsageErrorf(c, "products content get outputs a rich content JSON document; omit --plain or use --jq to filter it")
			}
			if err := validateProductContentVariantFlags(c, variantID, categoryID); err != nil {
				return err
			}
			selectedPageID, err := normalizeProductContentPageFlag(c, pageID)
			if err != nil {
				return err
			}

			requestOpts := opts
			if opts.UsesJSONOutput() {
				requestOpts.JSONOutput = false
				requestOpts.JQExpr = ""
			}

			productID := args[0]
			return cmdutil.Run(requestOpts, "Fetching content...", func(client *api.Client) (json.RawMessage, error) {
				_, richContent, err := fetchTargetProductRichContent(client, productID, variantID, categoryID)
				if err != nil {
					return nil, err
				}
				if selectedPageID != "" {
					page, err := selectRichContentPage(richContent, selectedPageID)
					if err != nil {
						return nil, err
					}
					return page, nil
				}
				return richContent, nil
			}, func(data json.RawMessage) error {
				return output.PrintJSON(opts.Out(), data, opts.JQExpr)
			})
		},
	}

	cmd.Flags().StringVar(&variantID, "variant", "", "Variant ID for per-variant content")
	cmd.Flags().StringVar(&categoryID, "category", "", "Variant category ID for per-variant content")
	cmd.Flags().StringVar(&pageID, "page", "", "Rich content page ID to dump")

	return cmd
}
