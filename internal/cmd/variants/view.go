package variants

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	var product, category string

	cmd := &cobra.Command{
		Use:   "view <variant_id>",
		Short: "View a variant",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if category == "" {
				return cmdutil.MissingFlagError(c, "--category")
			}

			path := cmdutil.JoinPath("products", product, "variant_categories", category, "variants", args[0])
			return cmdutil.RunRequest(opts, "Fetching variant...", "GET", path, url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					Variant struct {
						ID                   string      `json:"id"`
						Name                 string      `json:"name"`
						Description          string      `json:"description"`
						PriceDifferenceCents api.JSONInt `json:"price_difference_cents"`
						MaxPurchaseCount     api.JSONInt `json:"max_purchase_count"`
					} `json:"variant"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				v := resp.Variant
				style := opts.Style()

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{v.ID, v.Name, fmt.Sprintf("%d", v.PriceDifferenceCents)},
					})
				}

				if err := output.Writeln(opts.Out(), style.Bold(v.Name)); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "ID: %s\n", v.ID); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Price difference: %d cents\n", v.PriceDifferenceCents); err != nil {
					return err
				}
				if v.MaxPurchaseCount > 0 {
					if err := output.Writef(opts.Out(), "Max purchases: %d\n", v.MaxPurchaseCount); err != nil {
						return err
					}
				}
				if v.Description != "" {
					return output.Writeln(opts.Out(), style.Dim("\n"+v.Description))
				}

				return nil
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&category, "category", "", "Variant category ID (required)")

	return cmd
}
