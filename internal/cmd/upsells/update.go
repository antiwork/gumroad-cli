package upsells

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/spf13/cobra"
)

var updateFields = []string{
	"name", "product", "text", "description", "cross-sell", "universal",
	"replace-selected-products", "paused", "variant", "selected-product",
	"amount", "percent-off", "offer-variant", "remove-offer",
}

func newUpdateCmd() *cobra.Command {
	var name, product, text, description, variant, amount string
	var selectedProducts, offerVariants []string
	var percentOff int
	var crossSell, universal, replaceSelectedProducts, paused, removeOffer bool

	cmd := &cobra.Command{
		Use:   "update <upsell_id>",
		Short: "Update an upsell or cross-sell",
		Long: `Update an upsell or cross-sell.

Only the fields you pass are changed; everything else keeps its current value.
Pass --remove-offer to drop the discount. Passing --selected-product or
--offer-variant replaces the existing set.`,
		Example: `  gumroad upsells update <id> --name "New name"
  gumroad upsells update <id> --percent-off 25
  gumroad upsells update <id> --paused=false`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			id := args[0]

			if err := cmdutil.RequireAnyFlagChanged(c, updateFields...); err != nil {
				return err
			}
			if err := cmdutil.RequirePercentFlag(c, "percent-off", percentOff); err != nil {
				return err
			}

			offerCode, hasOfferCode, err := offerCodeFromFlags(c, amount, percentOff)
			if err != nil {
				return err
			}
			if removeOffer && hasOfferCode {
				return cmdutil.UsageErrorf(c, "--remove-offer cannot be used with --amount or --percent-off")
			}
			parsedVariants, err := parseOfferVariants(c, offerVariants)
			if err != nil {
				return err
			}

			token, err := config.Token()
			if err != nil {
				return err
			}

			current, err := fetchUpsell(opts, token, id)
			if err != nil {
				return err
			}

			flags := c.Flags()
			finalCrossSell := current.CrossSell
			if flags.Changed("cross-sell") {
				finalCrossSell = crossSell
			}
			finalUniversal := current.Universal
			if flags.Changed("universal") {
				finalUniversal = universal
			} else if flags.Changed("selected-product") {
				finalUniversal = false
			}
			if err := validateFlagConsistency(c, finalCrossSell, finalUniversal); err != nil {
				return err
			}

			body := currentUpsellBody(current)
			if flags.Changed("name") {
				body["name"] = name
			}
			if flags.Changed("product") {
				body["product_id"] = product
			}
			if flags.Changed("text") {
				body["text"] = text
			}
			if flags.Changed("description") {
				body["description"] = description
			}
			if flags.Changed("cross-sell") {
				body["cross_sell"] = crossSell
			}
			if flags.Changed("universal") {
				body["universal"] = universal
			}
			if flags.Changed("replace-selected-products") {
				body["replace_selected_products"] = replaceSelectedProducts
			}
			if flags.Changed("paused") {
				body["paused"] = paused
			}
			if flags.Changed("variant") {
				body["variant_id"] = variant
			} else if flags.Changed("product") {
				body["variant_id"] = ""
			}
			if flags.Changed("selected-product") {
				body["product_ids"] = nonEmptyValues(selectedProducts)
			}
			if flags.Changed("offer-variant") {
				body["upsell_variants"] = parsedVariants
			}
			if flags.Changed("selected-product") && !flags.Changed("universal") {
				body["universal"] = false
			}
			if finalCrossSell {
				if !flags.Changed("offer-variant") {
					body["upsell_variants"] = []map[string]any{}
				}
				if body["universal"] == true {
					body["product_ids"] = []string{}
				}
			} else {
				body["variant_id"] = ""
				body["product_ids"] = []string{}
				body["universal"] = false
				body["replace_selected_products"] = false
				if flags.Changed("product") && !flags.Changed("offer-variant") {
					body["upsell_variants"] = []map[string]any{}
				}
			}
			if hasOfferCode {
				body["offer_code"] = offerCode
			}
			if removeOffer {
				body["offer_code"] = map[string]any{}
			}

			return runUpsellWrite(opts, http.MethodPut, cmdutil.JoinPath("upsells", id), body, "Updating upsell...", "Updated")
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Upsell name")
	cmd.Flags().StringVar(&product, "product", "", "Offered product ID")
	cmd.Flags().BoolVar(&crossSell, "cross-sell", false, "Whether this is a cross-sell")
	cmd.Flags().StringVar(&text, "text", "", "Headline shown to the buyer")
	cmd.Flags().StringVar(&description, "description", "", "Description shown to the buyer")
	cmd.Flags().StringVar(&variant, "variant", "", "Offered version ID of the offered product (cross-sell)")
	cmd.Flags().BoolVar(&universal, "universal", false, "Offer the cross-sell to buyers of every product")
	cmd.Flags().BoolVar(&replaceSelectedProducts, "replace-selected-products", false, "Replace the selected products in the cart with the offered product")
	cmd.Flags().BoolVar(&paused, "paused", false, "Pause or unpause the upsell")
	cmd.Flags().StringVar(&amount, "amount", "", "Flat discount on the offered product (e.g. 5, 5.00)")
	cmd.Flags().IntVar(&percentOff, "percent-off", 0, "Percentage discount on the offered product")
	cmd.Flags().BoolVar(&removeOffer, "remove-offer", false, "Remove the discount from the upsell")
	cmd.Flags().StringArrayVar(&selectedProducts, "selected-product", nil, "Product ID whose buyers see the cross-sell (repeatable, replaces the current set)")
	cmd.Flags().StringArrayVar(&offerVariants, "offer-variant", nil, "Version upgrade as <selected_variant_id>:<offered_variant_id> (repeatable, replaces the current set)")

	return cmd
}

func fetchUpsell(opts cmdutil.Options, token, id string) (upsell, error) {
	data, err := cmdutil.RunWithTokenData(opts, token, "Fetching upsell...",
		func(client *api.Client) (json.RawMessage, error) {
			return client.Get(cmdutil.JoinPath("upsells", id), url.Values{})
		})
	if err != nil {
		return upsell{}, err
	}
	resp, err := cmdutil.DecodeJSON[upsellResponse](data)
	if err != nil {
		return upsell{}, err
	}
	return resp.Upsell, nil
}

func nonEmptyValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func currentUpsellBody(u upsell) map[string]any {
	body := map[string]any{
		"name":                      u.Name,
		"product_id":                u.Product.ID,
		"text":                      u.Text,
		"description":               u.Description,
		"cross_sell":                u.CrossSell,
		"universal":                 u.Universal,
		"replace_selected_products": u.ReplaceSelectedProducts,
		"paused":                    u.Paused,
	}
	if u.Product.Variant != nil {
		body["variant_id"] = u.Product.Variant.ID
	}
	if len(u.SelectedProducts) > 0 {
		ids := make([]string, len(u.SelectedProducts))
		for i, p := range u.SelectedProducts {
			ids[i] = p.ID
		}
		body["product_ids"] = ids
	}
	if u.Discount != nil {
		if u.Discount.Type == "fixed" {
			body["offer_code"] = map[string]any{"amount_cents": int(u.Discount.Cents)}
		} else {
			body["offer_code"] = map[string]any{"amount_percentage": int(u.Discount.Percents)}
		}
	}
	if len(u.UpsellVariants) > 0 {
		variants := make([]map[string]any, len(u.UpsellVariants))
		for i, v := range u.UpsellVariants {
			variants[i] = map[string]any{
				"selected_variant_id": v.SelectedVariant.ID,
				"offered_variant_id":  v.OfferedVariant.ID,
			}
		}
		body["upsell_variants"] = variants
	}
	return body
}
