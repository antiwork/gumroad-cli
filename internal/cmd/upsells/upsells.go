package upsells

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type variantRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type upsellVariant struct {
	ID              string     `json:"id"`
	SelectedVariant variantRef `json:"selected_variant"`
	OfferedVariant  variantRef `json:"offered_variant"`
}

type upsellProduct struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	CurrencyType string      `json:"currency_type"`
	Variant      *variantRef `json:"variant"`
}

type upsellDiscount struct {
	Type     string      `json:"type"`
	Cents    api.JSONInt `json:"cents"`
	Percents api.JSONInt `json:"percents"`
}

type selectedProduct struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type upsell struct {
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	CrossSell               bool              `json:"cross_sell"`
	ReplaceSelectedProducts bool              `json:"replace_selected_products"`
	Universal               bool              `json:"universal"`
	Text                    string            `json:"text"`
	Description             string            `json:"description"`
	Paused                  bool              `json:"paused"`
	Product                 upsellProduct     `json:"product"`
	Discount                *upsellDiscount   `json:"discount"`
	SelectedProducts        []selectedProduct `json:"selected_products"`
	UpsellVariants          []upsellVariant   `json:"upsell_variants"`
}

type upsellResponse struct {
	Upsell upsell `json:"upsell"`
}

func NewUpsellsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upsells",
		Aliases: []string{"upsell"},
		Short:   "Manage upsells and cross-sells",
		Long: `Manage upsells and cross-sells.

An upsell offers a buyer a different version of the product they are buying.
A cross-sell offers a different product (optionally discounted) to buyers of
selected products, or to buyers of every product when it is universal.`,
		Example: `  gumroad upsells list
  gumroad upsells create --name "Upgrade" --product <id> --offer-variant <selected_id>:<offered_id>
  gumroad upsells create --name "Audiobook" --product <id> --cross-sell --selected-product <id> --percent-off 50`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}

func upsellType(u upsell) string {
	if u.CrossSell {
		return "cross-sell"
	}
	return "upsell"
}

func formatDiscount(discount *upsellDiscount) string {
	if discount == nil {
		return "none"
	}
	if discount.Type == "fixed" {
		return fmt.Sprintf("%d cents off", discount.Cents)
	}
	return fmt.Sprintf("%d%% off", discount.Percents)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func runUpsellWrite(opts cmdutil.Options, method, path string, body map[string]any, spinnerMessage, verb string) error {
	if opts.DryRun {
		return printUpsellDryRun(opts, method, path, body)
	}

	return cmdutil.RunDecoded[upsellResponse](opts, spinnerMessage,
		func(client *api.Client) (json.RawMessage, error) {
			if method == http.MethodPut {
				return client.PutJSON(path, body)
			}
			return client.PostJSON(path, body)
		},
		func(resp upsellResponse) error {
			return renderUpsellWriteResult(opts, verb, resp.Upsell)
		})
}

func renderUpsellWriteResult(opts cmdutil.Options, verb string, u upsell) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{u.ID, u.Name}})
	}
	if opts.Quiet {
		return nil
	}
	s := opts.Style()
	return output.Writef(opts.Out(), "%s %s (%s)\n", s.Bold(verb+" upsell:"), u.Name, s.Dim(u.ID))
}

func printUpsellDryRun(opts cmdutil.Options, method, path string, body map[string]any) error {
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(map[string]any{
			"dry_run": true,
			"method":  method,
			"path":    path,
			"body":    body,
		})
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}

	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode dry-run output: %w", err)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{method, path, string(encoded)}})
	}
	if err := output.Writeln(opts.Out(), opts.Style().Yellow("Dry run")+": "+method+" "+path); err != nil {
		return err
	}
	return output.Writeln(opts.Out(), string(encoded))
}

func offerCodeFromFlags(c *cobra.Command, amount string, percentOff int) (map[string]any, bool, error) {
	flags := c.Flags()
	hasAmount := flags.Changed("amount")
	hasPercentOff := flags.Changed("percent-off")

	if hasAmount && hasPercentOff {
		return nil, false, cmdutil.UsageErrorf(c, "flags --amount and --percent-off cannot be used together")
	}
	if hasAmount {
		cents, err := cmdutil.ParseMoney("amount", amount, "amount", "")
		if err != nil {
			return nil, false, cmdutil.UsageErrorf(c, "%s", err.Error())
		}
		if cents <= 0 {
			return nil, false, cmdutil.UsageErrorf(c, "--amount must be greater than 0")
		}
		return map[string]any{"amount_cents": cents}, true, nil
	}
	if hasPercentOff {
		return map[string]any{"amount_percentage": percentOff}, true, nil
	}
	return nil, false, nil
}

func parseOfferVariants(c *cobra.Command, raw []string) ([]map[string]any, error) {
	variants := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		selected, offered, ok := strings.Cut(entry, ":")
		if !ok || selected == "" || offered == "" {
			return nil, cmdutil.UsageErrorf(c, "--offer-variant %q must be in the form <selected_variant_id>:<offered_variant_id>", entry)
		}
		variants = append(variants, map[string]any{
			"selected_variant_id": selected,
			"offered_variant_id":  offered,
		})
	}
	return variants, nil
}
