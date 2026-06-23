package upsells

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <upsell_id>",
		Short: "View an upsell or cross-sell",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			return cmdutil.RunRequest(opts, "Fetching upsell...", http.MethodGet, cmdutil.JoinPath("upsells", args[0]), url.Values{}, func(data json.RawMessage) error {
				var resp upsellResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}
				return renderUpsellDetail(opts, resp.Upsell)
			})
		},
	}
}

func renderUpsellDetail(opts cmdutil.Options, u upsell) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			u.ID,
			u.Name,
			upsellType(u),
			u.Product.Name,
			formatDiscount(u.Discount),
			yesNo(u.Paused),
		}})
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold(u.Name)); err != nil {
		return err
	}
	lines := [][2]string{
		{"ID", u.ID},
		{"Type", upsellType(u)},
		{"Offered product", productLabel(u.Product)},
	}
	if u.Product.Variant != nil {
		lines = append(lines, [2]string{"Offered version", u.Product.Variant.Name})
	}
	lines = append(lines,
		[2]string{"Discount", formatDiscount(u.Discount)},
		[2]string{"Universal", yesNo(u.Universal)},
		[2]string{"Replaces selected products", yesNo(u.ReplaceSelectedProducts)},
		[2]string{"Paused", yesNo(u.Paused)},
	)
	if u.Text != "" {
		lines = append(lines, [2]string{"Text", u.Text})
	}
	if u.Description != "" {
		lines = append(lines, [2]string{"Description", u.Description})
	}
	for _, line := range lines {
		if err := output.Writef(opts.Out(), "%s: %s\n", line[0], line[1]); err != nil {
			return err
		}
	}

	if len(u.SelectedProducts) > 0 {
		names := make([]string, len(u.SelectedProducts))
		for i, p := range u.SelectedProducts {
			names[i] = fmt.Sprintf("%s (%s)", p.Name, p.ID)
		}
		if err := output.Writef(opts.Out(), "Selected products: %s\n", strings.Join(names, ", ")); err != nil {
			return err
		}
	}

	if len(u.UpsellVariants) > 0 {
		if err := output.Writeln(opts.Out(), "Version upgrades:"); err != nil {
			return err
		}
		for _, v := range u.UpsellVariants {
			if err := output.Writef(opts.Out(), "  %s -> %s\n", v.SelectedVariant.Name, v.OfferedVariant.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func productLabel(p upsellProduct) string {
	return fmt.Sprintf("%s (%s)", p.Name, p.ID)
}
