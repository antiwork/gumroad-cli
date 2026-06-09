package products

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type productContentSetDryRun struct {
	DryRun         bool                `json:"dry_run"`
	Source         string              `json:"source,omitempty"`
	DeletedPageIDs []string            `json:"deleted_page_ids,omitempty"`
	Request        dryRunCreateRequest `json:"request"`
}

func newContentSetCmd() *cobra.Command {
	var variantID, categoryID string

	cmd := &cobra.Command{
		Use:   "set <product_id> [path|-]",
		Short: "Replace product rich content JSON",
		Long: "Replace a product's rich content page array from a JSON file or stdin.\n\n" +
			"This is a whole-document write. Existing pages omitted from the JSON are deleted, so run `--dry-run` before writing and pass `--yes` when you intend to delete omitted pages.",
		Args: productContentSetArgs,
		Example: `  gumroad products content set <product_id> content.json --dry-run
  gumroad products content set <product_id> content.json --variant <variant_id> --category <cat_id> --dry-run
  gumroad products content set <product_id> content.json --yes
  gumroad products content set <product_id> - < content.json`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID := args[0]
			if err := validateProductContentVariantFlags(c, variantID, categoryID); err != nil {
				return err
			}

			input, err := readProductContentInput(opts.In(), productContentPath(args))
			if err != nil {
				return cmdutil.InvalidInputErrorf("%s", err)
			}

			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)

			state, err := fetchProductContentState(client, productID)
			if err != nil {
				return err
			}
			target, err := resolveProductContentTarget(productID, state, variantID, categoryID)
			if err != nil {
				return err
			}
			existingRawRichContent := state.RichContent
			if target.usesVariant() {
				variantState, err := fetchVariantContentState(client, target.Path)
				if err != nil {
					return err
				}
				existingRawRichContent = variantState.RichContent
			}
			existingRichContent, err := normalizeProductRichContent(existingRawRichContent)
			if err != nil {
				return err
			}
			deletedIDs, err := deletedRichContentPageIDs(existingRichContent, input.RichContent)
			if err != nil {
				return err
			}

			ok, err := confirmProductContentDeletion(opts, target, deletedIDs)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "set content for "+target.confirmationSubject(), productID)
			}

			body := map[string]any{"rich_content": input.RichContent}
			if opts.DryRun {
				return renderProductContentSetDryRun(opts, target.Path, input.Source, deletedIDs, body)
			}

			data, err := runContentSetJSONData(opts, client, target.Path, body)
			if err != nil {
				return err
			}
			if target.usesVariant() {
				return cmdutil.PrintMutationSuccess(opts, data, target.VariantID, "Variant content updated.")
			}
			return cmdutil.PrintMutationSuccess(opts, data, productID, "Product content updated.")
		},
	}

	cmd.Flags().StringVar(&variantID, "variant", "", "Variant ID for per-variant content")
	cmd.Flags().StringVar(&categoryID, "category", "", "Variant category ID for per-variant content")

	return cmd
}

func confirmProductContentDeletion(opts cmdutil.Options, target productContentTarget, deletedIDs []string) (bool, error) {
	if len(deletedIDs) == 0 {
		return true, nil
	}
	return cmdutil.ConfirmAction(opts, productContentDeletionMessage(target, deletedIDs))
}

func productContentDeletionMessage(target productContentTarget, deletedIDs []string) string {
	subject := target.confirmationSubject()
	if len(deletedIDs) == 1 {
		return fmt.Sprintf("Set content for %s and delete rich content page %s?", subject, deletedIDs[0])
	}
	return fmt.Sprintf("Set content for %s and delete %d rich content pages: %s?", subject, len(deletedIDs), summarizeRichContentPageIDs(deletedIDs, 5))
}

func summarizeRichContentPageIDs(ids []string, max int) string {
	if max <= 0 || max > len(ids) {
		max = len(ids)
	}
	parts := append([]string(nil), ids[:max]...)
	if extra := len(ids) - max; extra > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", extra))
	}
	return strings.Join(parts, ", ")
}

func renderProductContentSetDryRun(
	opts cmdutil.Options,
	path, source string,
	deletedIDs []string,
	body map[string]any,
) error {
	payload := productContentSetDryRun{
		DryRun:         true,
		Source:         source,
		DeletedPageIDs: deletedIDs,
		Request: dryRunCreateRequest{
			Method: http.MethodPut,
			Path:   path,
			Body:   body,
		},
	}

	switch {
	case opts.UsesJSONOutput():
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	case opts.PlainOutput:
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintPlain(opts.Out(), [][]string{{
			http.MethodPut,
			path,
			string(data),
		}})
	default:
		style := opts.Style()
		if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": "+http.MethodPut+" "+path); err != nil {
			return err
		}
		if len(deletedIDs) > 0 {
			if err := output.Writeln(opts.Out(), "Deletes rich content pages: "+summarizeRichContentPageIDs(deletedIDs, 5)); err != nil {
				return err
			}
		}
		data, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.Writeln(opts.Out(), string(data))
	}
}

func runContentSetJSONData(
	opts cmdutil.Options,
	client *api.Client,
	path string,
	body map[string]any,
) (json.RawMessage, error) {
	var sp *output.Spinner
	if cmdutil.ShouldShowSpinner(opts) {
		sp = output.NewSpinnerTo("Updating content...", opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	return client.PutJSON(path, body)
}
