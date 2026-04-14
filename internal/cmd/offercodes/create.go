package offercodes

import (
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type createOfferCodeResponse struct {
	OfferCode struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"offer_code"`
}

func newCreateCmd() *cobra.Command {
	var product, name, amount string
	var percentOff, maxPurchaseCount int
	var universal bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an offer code",
		Args:  cmdutil.ExactArgs(0),
		Long: `Create an offer code for a product.

Use either --amount (flat discount) or --percent-off (percentage discount), not both.`,
		RunE: func(c *cobra.Command, args []string) error {
			if err := cmdutil.RequirePercentFlag(c, "percent-off", percentOff); err != nil {
				return err
			}
			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}

			flags := c.Flags()
			hasAmount := flags.Changed("amount")
			hasPercentOff := flags.Changed("percent-off")
			hasMaxPurchaseCount := flags.Changed("max-purchase-count")

			if hasAmount && hasPercentOff {
				return cmdutil.UsageErrorf(c, "flags --amount and --percent-off cannot be used together")
			}
			if !hasAmount && !hasPercentOff {
				return cmdutil.UsageErrorf(c, "one of --amount or --percent-off is required")
			}

			params := url.Values{}
			params.Set("name", name)
			if hasAmount {
				cents, err := cmdutil.ParseMoney("amount", amount, "amount", "")
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				if cents <= 0 {
					return cmdutil.UsageErrorf(c, "--amount must be greater than 0")
				}
				params.Set("amount_off", strconv.Itoa(cents))
			}
			if hasPercentOff {
				params.Set("amount_off", strconv.Itoa(percentOff))
				params.Set("offer_type", "percent")
			}
			if hasMaxPurchaseCount {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}
			if universal {
				params.Set("universal", "true")
			}

			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[createOfferCodeResponse](opts,
				"Creating offer code...", "POST", cmdutil.JoinPath("products", product, "offer_codes"), params,
				func(resp createOfferCodeResponse) error {
					oc := resp.OfferCode
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{oc.ID, oc.Name}})
					}
					if opts.Quiet {
						return nil
					}
					s := opts.Style()
					return output.Writef(opts.Out(), "%s %s (%s)\n",
						s.Bold("Created offer code:"), oc.Name, s.Dim(oc.ID))
				})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Offer code name (required)")
	cmd.Flags().StringVar(&amount, "amount", "", "Flat discount amount (e.g. 5, 5.00)")
	cmd.Flags().IntVar(&percentOff, "percent-off", 0, "Percentage discount")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "Maximum number of uses")
	cmd.Flags().BoolVar(&universal, "universal", false, "Universal offer code")

	return cmd
}
