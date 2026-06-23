package upsells

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var name, product, text, description, variant, amount string
	var selectedProducts, offerVariants []string
	var percentOff int
	var crossSell, universal, replaceSelectedProducts, paused bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an upsell or cross-sell",
		Long: `Create an upsell or cross-sell.

--product is the product being offered. For a version upgrade (an upsell), pass
one or more --offer-variant <selected_variant_id>:<offered_variant_id> pairs.

For a cross-sell, pass --cross-sell and either --universal (offer it to buyers of
every product) or one or more --selected-product <id> (offer it to buyers of those
products). Add an optional discount with --amount or --percent-off.`,
		Example: `  gumroad upsells create --name "Pro upgrade" --product <id> --offer-variant <selected_id>:<offered_id>
  gumroad upsells create --name "Audiobook" --product <id> --cross-sell --selected-product <id> --percent-off 50
  gumroad upsells create --name "Add-on" --product <id> --cross-sell --universal --amount 5`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if err := cmdutil.RequirePercentFlag(c, "percent-off", percentOff); err != nil {
				return err
			}

			offerCode, hasOfferCode, err := offerCodeFromFlags(c, amount, percentOff)
			if err != nil {
				return err
			}
			upsellVariants, err := parseOfferVariants(c, offerVariants)
			if err != nil {
				return err
			}

			flags := c.Flags()
			body := map[string]any{
				"name":       name,
				"product_id": product,
				"cross_sell": crossSell,
			}
			if flags.Changed("text") {
				body["text"] = text
			}
			if flags.Changed("description") {
				body["description"] = description
			}
			if flags.Changed("variant") {
				body["variant_id"] = variant
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
			if !universal {
				if ids := nonEmptyValues(selectedProducts); len(ids) > 0 {
					body["product_ids"] = ids
				}
			}
			if hasOfferCode {
				body["offer_code"] = offerCode
			}
			if len(upsellVariants) > 0 {
				body["upsell_variants"] = upsellVariants
			}

			return runUpsellWrite(opts, http.MethodPost, "/upsells", body, "Creating upsell...", "Created")
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Upsell name (required)")
	cmd.Flags().StringVar(&product, "product", "", "Offered product ID (required)")
	cmd.Flags().BoolVar(&crossSell, "cross-sell", false, "Create a cross-sell instead of a version upsell")
	cmd.Flags().StringVar(&text, "text", "", "Headline shown to the buyer")
	cmd.Flags().StringVar(&description, "description", "", "Description shown to the buyer")
	cmd.Flags().StringVar(&variant, "variant", "", "Offered version ID of the offered product (cross-sell)")
	cmd.Flags().BoolVar(&universal, "universal", false, "Offer the cross-sell to buyers of every product")
	cmd.Flags().BoolVar(&replaceSelectedProducts, "replace-selected-products", false, "Replace the selected products in the cart with the offered product")
	cmd.Flags().BoolVar(&paused, "paused", false, "Create the upsell paused")
	cmd.Flags().StringVar(&amount, "amount", "", "Flat discount on the offered product (e.g. 5, 5.00)")
	cmd.Flags().IntVar(&percentOff, "percent-off", 0, "Percentage discount on the offered product")
	cmd.Flags().StringArrayVar(&selectedProducts, "selected-product", nil, "Product ID whose buyers see the cross-sell (repeatable)")
	cmd.Flags().StringArrayVar(&offerVariants, "offer-variant", nil, "Version upgrade as <selected_variant_id>:<offered_variant_id> (repeatable)")

	return cmd
}
