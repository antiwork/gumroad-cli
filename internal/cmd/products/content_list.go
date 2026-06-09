package products

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type productContentPageSummary struct {
	ID         string       `json:"id,omitempty"`
	Title      string       `json:"title,omitempty"`
	Position   *api.JSONInt `json:"position,omitempty"`
	BlockCount int          `json:"block_count"`
}

func newContentListCmd() *cobra.Command {
	var variantID, categoryID string

	cmd := &cobra.Command{
		Use:   "list <product_id>",
		Short: "List product rich content pages",
		Long: "List product rich content pages.\n\n" +
			"Shows page metadata only: id, title, position, and top-level block count. It does not interpret rich content block types.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad products content list <product_id>
  gumroad products content list <product_id> --variant <variant_id> --category <cat_id>
  gumroad products content list <product_id> --json --jq '.[].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := validateProductContentVariantFlags(c, variantID, categoryID); err != nil {
				return err
			}

			productID := args[0]
			return cmdutil.Run(opts, "Fetching content...", func(client *api.Client) (json.RawMessage, error) {
				_, richContent, err := fetchTargetProductRichContent(client, productID, variantID, categoryID)
				if err != nil {
					return nil, err
				}
				summaries, err := summarizeRichContentPages(richContent)
				if err != nil {
					return nil, err
				}
				data, err := json.Marshal(summaries)
				if err != nil {
					return nil, fmt.Errorf("could not encode rich content page summaries: %w", err)
				}
				return data, nil
			}, func(data json.RawMessage) error {
				var summaries []productContentPageSummary
				if err := json.Unmarshal(data, &summaries); err != nil {
					return fmt.Errorf("could not parse rich content page summaries: %w", err)
				}
				return renderProductContentList(opts, summaries)
			})
		},
	}

	cmd.Flags().StringVar(&variantID, "variant", "", "Variant ID for per-variant content")
	cmd.Flags().StringVar(&categoryID, "category", "", "Variant category ID for per-variant content")

	return cmd
}

func summarizeRichContentPages(richContent json.RawMessage) ([]productContentPageSummary, error) {
	pages, err := decodeRichContentPages(richContent)
	if err != nil {
		return nil, err
	}

	summaries := make([]productContentPageSummary, 0, len(pages))
	for idx, page := range pages {
		var parsed struct {
			ID          string       `json:"id"`
			Title       string       `json:"title"`
			Position    *api.JSONInt `json:"position"`
			Description struct {
				Content []json.RawMessage `json:"content"`
			} `json:"description"`
		}
		if err := json.Unmarshal(page, &parsed); err != nil {
			return nil, fmt.Errorf("rich content page %d is invalid: %w", idx, err)
		}
		summaries = append(summaries, productContentPageSummary{
			ID:         parsed.ID,
			Title:      parsed.Title,
			Position:   parsed.Position,
			BlockCount: len(parsed.Description.Content),
		})
	}
	return summaries, nil
}

func renderProductContentList(opts cmdutil.Options, summaries []productContentPageSummary) error {
	if len(summaries) == 0 {
		return cmdutil.PrintInfo(opts, "No rich content pages found.")
	}

	if opts.PlainOutput {
		rows := make([][]string, 0, len(summaries))
		for _, page := range summaries {
			rows = append(rows, []string{
				page.ID,
				page.Title,
				formatRichContentPosition(page.Position),
				strconv.Itoa(page.BlockCount),
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	style := opts.Style()
	tbl := output.NewStyledTable(style, "ID", "TITLE", "POSITION", "BLOCKS")
	for _, page := range summaries {
		tbl.AddRow(page.ID, page.Title, formatRichContentPosition(page.Position), strconv.Itoa(page.BlockCount))
	}
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		return tbl.Render(w)
	})
}

func formatRichContentPosition(position *api.JSONInt) string {
	if position == nil {
		return ""
	}
	return strconv.Itoa(int(*position))
}
