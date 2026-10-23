package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type bundleItem struct {
	ProductID string `json:"product_id"`
	VariantID string `json:"variant_id,omitempty"`
	Quantity  int    `json:"quantity"`
	Position  int    `json:"position"`
}

type bundleItemResponse struct {
	ProductID string  `json:"product_id"`
	VariantID *string `json:"variant_id"`
	Quantity  int     `json:"quantity"`
	Position  *int    `json:"position"`
}

type bundleProductResponse struct {
	Success bool `json:"success"`
	Product struct {
		ID             string               `json:"id"`
		Name           string               `json:"name"`
		BundleProducts []bundleItemResponse `json:"bundle_products"`
	} `json:"product"`
}

type bundleSetDryRun struct {
	DryRun bool           `json:"dry_run"`
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body"`
}

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage the products inside a bundle",
		Long: "Manage the products inside a bundle.\n\n" +
			"Create the bundle first with `gumroad products create --type bundle`, then add its contents " +
			"with `gumroad products bundle set`. A bundle with no contents delivers nothing to buyers.",
		Example: `  gumroad products create --name "Starter Bundle" --type bundle --price 29
  gumroad products bundle set <bundle_id> --product <product_id> --product <product_id>
  gumroad products bundle list <bundle_id>`,
	}
	cmd.AddCommand(newBundleListCmd())
	cmd.AddCommand(newBundleSetCmd())
	return cmd
}

func newBundleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list <bundle_id>",
		Short:   "List the products inside a bundle",
		Args:    cmdutil.ExactArgs(1),
		Example: `  gumroad products bundle list <bundle_id>`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[bundleProductResponse](opts, "Fetching bundle...", http.MethodGet,
				cmdutil.JoinPath("products", args[0]), nil, func(resp bundleProductResponse) error {
					return renderBundleItems(opts, resp)
				})
		},
	}
}

func newBundleSetCmd() *cobra.Command {
	var productIDs, variants, quantities []string

	cmd := &cobra.Command{
		Use:   "set <bundle_id>",
		Short: "Replace the products inside a bundle",
		Long: "Replace the products inside a bundle.\n\n" +
			"This is a whole-list write: the bundle ends up containing exactly the --product values given, " +
			"in that order. Products already in the bundle but not listed are removed. " +
			"Run with --dry-run to see the request first.\n\n" +
			"Pin a version of a bundled product with --variant <product_id>=<variant_id>, and include more " +
			"than one copy with --quantity <product_id>=<n>.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad products bundle set <bundle_id> --product <id_a> --product <id_b>
  gumroad products bundle set <bundle_id> --product <id_a> --variant <id_a>=<variant_id>
  gumroad products bundle set <bundle_id> --product <id_a> --quantity <id_a>=2 --dry-run`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if len(productIDs) == 0 {
				return cmdutil.MissingFlagError(c, "--product")
			}
			items, err := buildBundleItems(productIDs, variants, quantities)
			if err != nil {
				return cmdutil.UsageErrorf(c, "%s", err.Error())
			}

			bundleID := args[0]
			path := cmdutil.JoinPath("products", bundleID, "bundle_contents")
			body := map[string]any{"products": items}

			if opts.DryRun {
				return renderBundleSetDryRun(opts, path, body)
			}

			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)
			var sp *output.Spinner
			if cmdutil.ShouldShowSpinner(opts) {
				sp = output.NewSpinnerTo("Updating bundle contents...", opts.Err())
				sp.Start()
			}
			data, err := client.PutJSON(path, body)
			if sp != nil {
				sp.Stop()
			}
			if err != nil {
				return err
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}
			var resp bundleProductResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("could not parse response: %w", err)
			}
			if !opts.PlainOutput && !opts.Quiet {
				if err := cmdutil.PrintSuccess(opts, fmt.Sprintf("Bundle %s now contains %d product(s).", bundleID, len(resp.Product.BundleProducts))); err != nil {
					return err
				}
			}
			return renderBundleItems(opts, resp)
		},
	}

	cmd.Flags().StringArrayVar(&productIDs, "product", nil, "Product ID to include (repeatable; order sets position)")
	cmd.Flags().StringArrayVar(&variants, "variant", nil, "Pin a version: <product_id>=<variant_id> (repeatable)")
	cmd.Flags().StringArrayVar(&quantities, "quantity", nil, "Copies of a product: <product_id>=<n> (repeatable, default 1)")
	return cmd
}

func buildBundleItems(productIDs, variants, quantities []string) ([]bundleItem, error) {
	items := make([]bundleItem, 0, len(productIDs))
	index := map[string]int{}
	for i, raw := range productIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("--product cannot be empty")
		}
		if _, dup := index[id]; dup {
			return nil, fmt.Errorf("--product %q given more than once; use --quantity %s=<n> for copies", id, id)
		}
		index[id] = i
		items = append(items, bundleItem{ProductID: id, Quantity: 1, Position: i})
	}

	for _, raw := range variants {
		id, value, err := splitBundlePair("--variant", raw, index)
		if err != nil {
			return nil, err
		}
		items[index[id]].VariantID = value
	}
	for _, raw := range quantities {
		id, value, err := splitBundlePair("--quantity", raw, index)
		if err != nil {
			return nil, err
		}
		n, convErr := strconv.Atoi(value)
		if convErr != nil || n < 1 {
			return nil, fmt.Errorf("--quantity for %s must be a whole number of at least 1, got %q", id, value)
		}
		items[index[id]].Quantity = n
	}
	return items, nil
}

func splitBundlePair(flag, raw string, index map[string]int) (string, string, error) {
	// Product IDs are base64 and may end in "=", so the separator is found by
	// matching a supplied --product value rather than the first '=' in raw.
	id, value, matched := "", "", false
	for known := range index {
		if !strings.HasPrefix(raw, known+"=") {
			continue
		}
		if !matched || len(known) > len(id) {
			id, value, matched = known, strings.TrimSpace(raw[len(known)+1:]), true
		}
	}
	if !matched {
		if guess, _, ok := strings.Cut(raw, "="); ok && guess != "" {
			return "", "", fmt.Errorf("%s refers to %s, which is not one of the --product values", flag, guess)
		}
		return "", "", fmt.Errorf("%s must look like <product_id>=<value>, got %q", flag, raw)
	}
	if value == "" {
		return "", "", fmt.Errorf("%s must look like <product_id>=<value>, got %q", flag, raw)
	}
	return id, value, nil
}

func renderBundleSetDryRun(opts cmdutil.Options, path string, body map[string]any) error {
	payload := bundleSetDryRun{DryRun: true, Method: http.MethodPut, Path: path, Body: body}
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}
	items, _ := body["products"].([]bundleItem)
	if opts.PlainOutput {
		rows := make([][]string, 0, len(items))
		for _, it := range items {
			rows = append(rows, []string{http.MethodPut, path, it.ProductID, it.VariantID, strconv.Itoa(it.Quantity)})
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": "+http.MethodPut+" "+path); err != nil {
		return err
	}
	for _, it := range items {
		line := fmt.Sprintf("  %d. %s x%d", it.Position+1, it.ProductID, it.Quantity)
		if it.VariantID != "" {
			line += " (variant " + it.VariantID + ")"
		}
		if err := output.Writeln(opts.Out(), line); err != nil {
			return err
		}
	}
	return nil
}

func renderBundleItems(opts cmdutil.Options, resp bundleProductResponse) error {
	items := resp.Product.BundleProducts
	if opts.PlainOutput {
		rows := make([][]string, 0, len(items))
		for _, it := range items {
			rows = append(rows, []string{it.ProductID, derefString(it.VariantID), strconv.Itoa(it.Quantity)})
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if opts.Quiet {
		return nil
	}
	if len(items) == 0 {
		return cmdutil.PrintInfo(opts, "This bundle has no products. Add some with: gumroad products bundle set "+resp.Product.ID+" --product <product_id>")
	}
	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		tbl := output.NewStyledTable(style, "PRODUCT", "VARIANT", "QTY")
		for _, it := range items {
			tbl.AddRow(output.EscapePlainField(it.ProductID), output.EscapePlainField(derefString(it.VariantID)), strconv.Itoa(it.Quantity))
		}
		return tbl.Render(w)
	})
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
