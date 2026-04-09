package categories

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type createCategoryResponse struct {
	VariantCategory struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"variant_category"`
}

func newCreateCmd() *cobra.Command {
	var product, title string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a variant category",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if title == "" {
				return cmdutil.MissingFlagError(c, "--title")
			}

			params := url.Values{}
			params.Set("title", title)

			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[createCategoryResponse](opts,
				"Creating variant category...", "POST", cmdutil.JoinPath("products", product, "variant_categories"), params,
				func(resp createCategoryResponse) error {
					vc := resp.VariantCategory
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{vc.ID, vc.Title}})
					}
					if opts.Quiet {
						return nil
					}
					s := opts.Style()
					return output.Writef(opts.Out(), "%s %s (%s)\n",
						s.Bold("Created variant category:"), vc.Title, s.Dim(vc.ID))
				})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&title, "title", "", "Category title (required)")

	return cmd
}
