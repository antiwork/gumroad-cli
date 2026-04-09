package products

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func sortedKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

type createProductResponse struct {
	Product struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		FormattedPrice string `json:"formatted_price"`
	} `json:"product"`
}

var validProductTypes = map[string]bool{
	"digital": true, "course": true, "ebook": true,
	"membership": true, "bundle": true, "coffee": true,
	"call": true, "commission": true,
}

var validSubscriptionDurations = map[string]bool{
	"monthly": true, "quarterly": true, "biannually": true,
	"yearly": true, "every_two_years": true,
}

func newCreateCmd() *cobra.Command {
	var name, nativeType, currency, description, customPermalink string
	var customSummary, customReceipt, subscriptionDuration, taxonomyID string
	var price, suggestedPrice string
	var maxPurchaseCount int
	var payWhatYouWant bool
	var tags []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new product (as draft)",
		Example: `  gumroad products create --name "Art Pack" --price 10.00
  gumroad products create --name "Newsletter" --type membership --subscription-duration monthly
  gumroad products create --name "E-Book" --type ebook --price 5 --tag art --tag digital`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			flags := c.Flags()

			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}

			if !validProductTypes[nativeType] {
				return cmdutil.UsageErrorf(c, "invalid --type %q; must be one of: %s", nativeType, sortedKeys(validProductTypes))
			}

			if flags.Changed("subscription-duration") {
				if nativeType != "membership" {
					return cmdutil.UsageErrorf(c, "--subscription-duration can only be used with --type membership")
				}
				if !validSubscriptionDurations[subscriptionDuration] {
					return cmdutil.UsageErrorf(c, "invalid --subscription-duration %q; must be one of: %s", subscriptionDuration, sortedKeys(validSubscriptionDurations))
				}
			}

			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}

			params := url.Values{}
			params.Set("name", name)
			params.Set("native_type", nativeType)
			currency = strings.ToLower(currency)
			if flags.Changed("price") {
				cents, err := cmdutil.ParseMoney("price", price, "price", currency)
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				params.Set("price", strconv.Itoa(cents))
			}
			if flags.Changed("currency") {
				params.Set("price_currency_type", currency)
			}
			if flags.Changed("description") {
				params.Set("description", description)
			}
			if flags.Changed("custom-permalink") {
				params.Set("custom_permalink", customPermalink)
			}
			if flags.Changed("custom-summary") {
				params.Set("custom_summary", customSummary)
			}
			if flags.Changed("custom-receipt") {
				params.Set("custom_receipt", customReceipt)
			}
			if flags.Changed("pay-what-you-want") {
				params.Set("customizable_price", strconv.FormatBool(payWhatYouWant))
			}
			if flags.Changed("suggested-price") {
				cents, err := cmdutil.ParseMoney("suggested-price", suggestedPrice, "suggested price", currency)
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				params.Set("suggested_price_cents", strconv.Itoa(cents))
			}
			if flags.Changed("max-purchase-count") {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}
			if flags.Changed("taxonomy-id") {
				params.Set("taxonomy_id", taxonomyID)
			}
			if flags.Changed("subscription-duration") {
				params.Set("subscription_duration", subscriptionDuration)
			}
			for _, t := range tags {
				params.Add("tags[]", t)
			}

			return cmdutil.RunRequestDecoded[createProductResponse](opts,
				"Creating product...", "POST", "/products", params,
				func(resp createProductResponse) error {
					p := resp.Product
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{
							{p.ID, p.Name, p.FormattedPrice},
						})
					}
					if opts.Quiet {
						return nil
					}
					s := opts.Style()
					if err := output.Writef(opts.Out(), "%s %s (%s)\n",
						s.Bold("Created draft product:"), p.Name, s.Dim(p.ID)); err != nil {
						return err
					}
					return output.Writef(opts.Out(), "\n%s gumroad products enable %s\n",
						s.Dim("Publish with:"), p.ID)
				})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Product name (required)")
	cmd.Flags().StringVar(&nativeType, "type", "digital", "Product type (digital, course, ebook, membership, bundle, coffee, call, commission)")
	cmd.Flags().StringVar(&price, "price", "", "Price (e.g. 10, 10.00, 9.99)")
	cmd.Flags().StringVar(&currency, "currency", "", "Price currency (e.g. usd, eur)")
	cmd.Flags().StringVar(&description, "description", "", "HTML description")
	cmd.Flags().StringVar(&customPermalink, "custom-permalink", "", "Custom URL slug")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "Short summary")
	cmd.Flags().StringVar(&customReceipt, "custom-receipt", "", "Custom receipt text")
	cmd.Flags().BoolVar(&payWhatYouWant, "pay-what-you-want", false, "Enable pay-what-you-want pricing")
	cmd.Flags().StringVar(&suggestedPrice, "suggested-price", "", "Suggested price for pay-what-you-want (e.g. 5, 5.00)")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "Maximum number of purchases (inventory limit)")
	cmd.Flags().StringVar(&taxonomyID, "taxonomy-id", "", "Taxonomy/category ID")
	cmd.Flags().StringVar(&subscriptionDuration, "subscription-duration", "", "Subscription duration (membership only: monthly, quarterly, biannually, yearly, every_two_years)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")

	return cmd
}
