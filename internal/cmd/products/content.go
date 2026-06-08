package products

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

const defaultProductContentPath = "./content.json"

type productContentState struct {
	RichContent                      json.RawMessage
	HasSameRichContentForAllVariants bool
	Variants                         *[]productVariantCategoryRef
}

type rawProductContentState struct {
	RichContent                      json.RawMessage              `json:"rich_content"`
	HasSameRichContentForAllVariants bool                         `json:"has_same_rich_content_for_all_variants"`
	Variants                         *[]productVariantCategoryRef `json:"variants"`
}

type productContentResponse struct {
	Product                          *rawProductContentState      `json:"product,omitempty"`
	RichContent                      json.RawMessage              `json:"rich_content,omitempty"`
	HasSameRichContentForAllVariants bool                         `json:"has_same_rich_content_for_all_variants,omitempty"`
	Variants                         *[]productVariantCategoryRef `json:"variants,omitempty"`
}

type productContentInput struct {
	Source      string
	RichContent json.RawMessage
}

func newContentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "content",
		Short: "Manage product rich content",
		Long: "Manage product rich content.\n\n" +
			"Get or replace the whole rich content document for products that use shared content.",
		Example: `  gumroad products content get <product_id> > content.json
  gumroad products content set <product_id> content.json --dry-run
  gumroad products content set <product_id> content.json --yes
  gumroad products content set <product_id> - < content.json`,
	}

	cmd.AddCommand(newContentGetCmd())
	cmd.AddCommand(newContentSetCmd())
	return cmd
}

func productContentPath(args []string) string {
	if len(args) > 1 {
		return args[1]
	}
	return defaultProductContentPath
}

func productContentSetArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return cmdutil.UsageErrorf(cmd, "missing required argument: <product_id>")
	}
	if len(args) > 2 {
		return cmdutil.UsageErrorf(cmd, "unexpected argument: %s", args[2])
	}
	return nil
}

func fetchProductContentState(client *api.Client, productID string) (productContentState, error) {
	data, err := client.Get(cmdutil.JoinPath("products", productID), url.Values{})
	if err != nil {
		return productContentState{}, err
	}

	resp, err := cmdutil.DecodeJSON[productContentResponse](data)
	if err != nil {
		return productContentState{}, err
	}
	return resp.state(), nil
}

func (r productContentResponse) state() productContentState {
	if r.Product != nil {
		return productContentState{
			RichContent:                      r.Product.RichContent,
			HasSameRichContentForAllVariants: r.Product.HasSameRichContentForAllVariants,
			Variants:                         r.Product.Variants,
		}
	}
	return productContentState{
		RichContent:                      r.RichContent,
		HasSameRichContentForAllVariants: r.HasSameRichContentForAllVariants,
		Variants:                         r.Variants,
	}
}

func ensureSharedProductContent(productID string, state productContentState) error {
	if !productUsesPerVariantRichContent(productFileUpdateState{
		HasSameRichContentForAllVariants: state.HasSameRichContentForAllVariants,
		Variants:                         state.Variants,
	}) {
		return nil
	}
	return cmdutil.InvalidInputErrorf("product %s uses per-variant rich content; product-level content get/set only supports shared rich content", productID)
}

func readProductContentInput(r io.Reader, path string) (productContentInput, error) {
	if path == "" {
		path = defaultProductContentPath
	}

	var (
		source string
		data   []byte
		err    error
	)
	if path == "-" {
		source = "stdin"
		data, err = io.ReadAll(r)
		if err != nil {
			return productContentInput{}, fmt.Errorf("cannot read stdin: %w", err)
		}
	} else {
		source = path
		data, err = os.ReadFile(path)
		if err != nil {
			return productContentInput{}, fmt.Errorf("cannot read %s: %w", path, err)
		}
	}

	richContent, err := parseProductContentDocument(data)
	if err != nil {
		return productContentInput{}, err
	}
	return productContentInput{Source: source, RichContent: richContent}, nil
}

func parseProductContentDocument(data []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("rich content JSON cannot be empty")
	}
	if err := validateRichContentArray(trimmed); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func normalizeProductRichContent(data json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage("[]"), nil
	}
	if err := validateRichContentArray(trimmed); err != nil {
		return nil, fmt.Errorf("product rich_content response is invalid: %w", err)
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func validateRichContentArray(data []byte) error {
	if !bytes.HasPrefix(data, []byte("[")) {
		return fmt.Errorf("rich content JSON must be an array")
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(data, &pages); err != nil {
		return fmt.Errorf("rich content JSON must be an array: %w", err)
	}
	for idx, page := range pages {
		if !bytes.HasPrefix(bytes.TrimSpace(page), []byte("{")) {
			return fmt.Errorf("rich content JSON page %d must be an object", idx)
		}
	}
	return nil
}

func deletedRichContentPageIDs(existing, next json.RawMessage) ([]string, error) {
	existingIDs, _, err := richContentPageIDs(existing)
	if err != nil {
		return nil, fmt.Errorf("existing rich_content is invalid: %w", err)
	}
	_, nextSet, err := richContentPageIDs(next)
	if err != nil {
		return nil, fmt.Errorf("new rich_content is invalid: %w", err)
	}

	var deleted []string
	for _, id := range existingIDs {
		if _, ok := nextSet[id]; !ok {
			deleted = append(deleted, id)
		}
	}
	return deleted, nil
}

func richContentPageIDs(data json.RawMessage) ([]string, map[string]struct{}, error) {
	var pages []json.RawMessage
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, nil, err
	}

	ordered := make([]string, 0, len(pages))
	seen := make(map[string]struct{}, len(pages))
	for idx, page := range pages {
		var parsed struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(page, &parsed); err != nil {
			return nil, nil, fmt.Errorf("page %d must be an object: %w", idx, err)
		}
		id := strings.TrimSpace(parsed.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	return ordered, seen, nil
}
