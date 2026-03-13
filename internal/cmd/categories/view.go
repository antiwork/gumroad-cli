package categories

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	var product string

	cmd := &cobra.Command{
		Use:   "view <category_id>",
		Short: "View a variant category",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			return cmdutil.RunRequest(opts, "Fetching variant category...", "GET", cmdutil.JoinPath("products", product, "variant_categories", args[0]), url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					VariantCategory struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"variant_category"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				vc := resp.VariantCategory
				style := opts.Style()

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{vc.ID, vc.Title},
					})
				}

				if err := output.Writeln(opts.Out(), style.Bold(vc.Title)); err != nil {
					return err
				}
				return output.Writef(opts.Out(), "ID: %s\n", vc.ID)
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")

	return cmd
}
