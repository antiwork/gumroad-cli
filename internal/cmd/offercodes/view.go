package offercodes

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
	var product string

	cmd := &cobra.Command{
		Use:   "view <code_id>",
		Short: "View an offer code",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			return cmdutil.RunRequest(opts, "Fetching offer code...", "GET", cmdutil.JoinPath("products", product, "offer_codes", args[0]), url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					OfferCode struct {
						ID               string      `json:"id"`
						Name             string      `json:"name"`
						AmountOff        api.JSONInt `json:"amount_off"`
						PercentOff       api.JSONInt `json:"percent_off"`
						MaxPurchaseCount api.JSONInt `json:"max_purchase_count"`
						Universal        bool        `json:"universal"`
					} `json:"offer_code"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				oc := resp.OfferCode
				style := opts.Style()

				if opts.PlainOutput {
					discount := fmt.Sprintf("%d%%", oc.PercentOff)
					if oc.AmountOff > 0 {
						discount = fmt.Sprintf("%d cents", oc.AmountOff)
					}
					return output.PrintPlain(opts.Out(), [][]string{
						{oc.ID, oc.Name, discount},
					})
				}

				if err := output.Writeln(opts.Out(), style.Bold(oc.Name)); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "ID: %s\n", oc.ID); err != nil {
					return err
				}
				if oc.AmountOff > 0 {
					if err := output.Writef(opts.Out(), "Discount: %d cents off\n", oc.AmountOff); err != nil {
						return err
					}
				} else {
					if err := output.Writef(opts.Out(), "Discount: %d%% off\n", oc.PercentOff); err != nil {
						return err
					}
				}
				if oc.MaxPurchaseCount > 0 {
					if err := output.Writef(opts.Out(), "Max uses: %d\n", oc.MaxPurchaseCount); err != nil {
						return err
					}
				}
				if oc.Universal {
					return output.Writeln(opts.Out(), "Universal: yes")
				}

				return nil
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")

	return cmd
}
