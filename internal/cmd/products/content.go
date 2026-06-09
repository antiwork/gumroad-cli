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

type variantContentState struct {
	RichContent json.RawMessage
}

type rawProductContentState struct {
	RichContent                      json.RawMessage              `json:"rich_content"`
	HasSameRichContentForAllVariants bool                         `json:"has_same_rich_content_for_all_variants"`
	Variants                         *[]productVariantCategoryRef `json:"variants"`
}

type rawVariantContentState struct {
	RichContent json.RawMessage `json:"rich_content"`
}

type productContentResponse struct {
	Product                          *rawProductContentState      `json:"product,omitempty"`
	RichContent                      json.RawMessage              `json:"rich_content,omitempty"`
	HasSameRichContentForAllVariants bool                         `json:"has_same_rich_content_for_all_variants,omitempty"`
	Variants                         *[]productVariantCategoryRef `json:"variants,omitempty"`
}

type variantContentResponse struct {
	Variant *rawVariantContentState `json:"variant,omitempty"`
}

type productContentInput struct {
	Source      string
	RichContent json.RawMessage
	Page        json.RawMessage
}

type productContentTarget struct {
	ProductID string
	VariantID string
	Path      string
}

func newContentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "content",
		Short: "Manage product rich content",
		Long: "Manage product rich content.\n\n" +
			"Get or replace the whole rich content document for products that use shared content or per-variant content.",
		Example: `  gumroad products content get <product_id> > content.json
  gumroad products content get <product_id> --variant <variant_id> --category <cat_id> > content.json
  gumroad products content get <product_id> --page <page_id> > page.json
  gumroad products content list <product_id>
  gumroad products content set <product_id> content.json --dry-run
  gumroad products content set <product_id> page.json --page <page_id> --dry-run
  gumroad products content set <product_id> content.json --variant <variant_id> --category <cat_id> --dry-run
  gumroad products content set <product_id> content.json --yes
  gumroad products content set <product_id> - < content.json`,
	}

	cmd.AddCommand(newContentGetCmd())
	cmd.AddCommand(newContentListCmd())
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

func fetchVariantContentState(client *api.Client, path string) (variantContentState, error) {
	data, err := client.Get(path, url.Values{})
	if err != nil {
		return variantContentState{}, err
	}

	resp, err := cmdutil.DecodeJSON[variantContentResponse](data)
	if err != nil {
		return variantContentState{}, err
	}
	if resp.Variant == nil {
		return variantContentState{}, fmt.Errorf("variant response is missing variant")
	}
	return variantContentState{
		RichContent: resp.Variant.RichContent,
	}, nil
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

func validateProductContentVariantFlags(cmd *cobra.Command, variantID, categoryID string) error {
	variantID = strings.TrimSpace(variantID)
	categoryID = strings.TrimSpace(categoryID)
	switch {
	case variantID == "" && categoryID == "":
		return nil
	case variantID == "":
		return cmdutil.UsageErrorf(cmd, "--category can only be used with --variant")
	case categoryID == "":
		return cmdutil.MissingFlagError(cmd, "--category")
	default:
		return nil
	}
}

func normalizeProductContentPageFlag(cmd *cobra.Command, pageID string) (string, error) {
	if cmd == nil || cmd.Flags() == nil || !cmd.Flags().Changed("page") {
		return "", nil
	}
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return "", cmdutil.MissingFlagError(cmd, "--page")
	}
	return pageID, nil
}

func resolveProductContentTarget(
	productID string,
	state productContentState,
	variantID, categoryID string,
) (productContentTarget, error) {
	variantID = strings.TrimSpace(variantID)
	categoryID = strings.TrimSpace(categoryID)
	if variantID == "" {
		if err := ensureSharedProductContent(productID, state); err != nil {
			return productContentTarget{}, err
		}
		return productContentTarget{
			ProductID: productID,
			Path:      cmdutil.JoinPath("products", productID),
		}, nil
	}

	if !productUsesPerVariantRichContent(productFileUpdateState{
		HasSameRichContentForAllVariants: state.HasSameRichContentForAllVariants,
		Variants:                         state.Variants,
	}) {
		return productContentTarget{}, cmdutil.InvalidInputErrorf(
			"product %s uses shared rich content; omit --variant to edit product-level content, or switch the product to per-variant content first",
			productID)
	}

	return productContentTarget{
		ProductID: productID,
		VariantID: variantID,
		Path:      cmdutil.JoinPath("products", productID, "variant_categories", categoryID, "variants", variantID),
	}, nil
}

func fetchTargetProductRichContent(
	client *api.Client,
	productID, variantID, categoryID string,
) (productContentTarget, json.RawMessage, error) {
	state, err := fetchProductContentState(client, productID)
	if err != nil {
		return productContentTarget{}, nil, err
	}
	target, err := resolveProductContentTarget(productID, state, variantID, categoryID)
	if err != nil {
		return productContentTarget{}, nil, err
	}

	rawRichContent := state.RichContent
	if target.usesVariant() {
		variantState, err := fetchVariantContentState(client, target.Path)
		if err != nil {
			return productContentTarget{}, nil, err
		}
		rawRichContent = variantState.RichContent
	}
	richContent, err := normalizeProductRichContent(rawRichContent)
	if err != nil {
		return productContentTarget{}, nil, err
	}
	return target, richContent, nil
}

func (t productContentTarget) usesVariant() bool {
	return t.VariantID != ""
}

func (t productContentTarget) mutationID() string {
	if t.usesVariant() {
		return t.VariantID
	}
	return t.ProductID
}

func (t productContentTarget) confirmationSubject() string {
	if t.usesVariant() {
		return fmt.Sprintf("variant %s for product %s", t.VariantID, t.ProductID)
	}
	return fmt.Sprintf("product %s", t.ProductID)
}

func ensureSharedProductContent(productID string, state productContentState) error {
	if !productUsesPerVariantRichContent(productFileUpdateState{
		HasSameRichContentForAllVariants: state.HasSameRichContentForAllVariants,
		Variants:                         state.Variants,
	}) {
		return nil
	}
	return cmdutil.InvalidInputErrorf("product %s uses per-variant rich content; pass --variant <variant_id> --category <cat_id> to edit variant content", productID)
}

func readProductContentInput(r io.Reader, path string) (productContentInput, error) {
	source, data, err := readProductContentSource(r, path)
	if err != nil {
		return productContentInput{}, err
	}

	richContent, err := parseProductContentDocument(data)
	if err != nil {
		return productContentInput{}, err
	}
	return productContentInput{Source: source, RichContent: richContent}, nil
}

func readProductContentPageInput(r io.Reader, path string) (productContentInput, error) {
	source, data, err := readProductContentSource(r, path)
	if err != nil {
		return productContentInput{}, err
	}

	page, err := parseProductContentPage(data)
	if err != nil {
		return productContentInput{}, err
	}
	return productContentInput{Source: source, Page: page}, nil
}

func readProductContentSource(r io.Reader, path string) (string, []byte, error) {
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
			return "", nil, fmt.Errorf("cannot read stdin: %w", err)
		}
	} else {
		source = path
		data, err = os.ReadFile(path)
		if err != nil {
			return "", nil, fmt.Errorf("cannot read %s: %w", path, err)
		}
	}
	return source, data, nil
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

func parseProductContentPage(data []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("rich content page JSON cannot be empty")
	}
	if !bytes.HasPrefix(trimmed, []byte("{")) {
		return nil, fmt.Errorf("rich content page JSON must be an object")
	}
	var page map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &page); err != nil {
		return nil, fmt.Errorf("rich content page JSON must be an object: %w", err)
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func normalizeProductRichContent(data json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage("[]"), nil
	}
	if err := validateRichContentArray(trimmed); err != nil {
		return nil, fmt.Errorf("rich_content response is invalid: %w", err)
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func selectRichContentPage(richContent json.RawMessage, pageID string) (json.RawMessage, error) {
	pages, err := decodeRichContentPages(richContent)
	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		id, err := richContentPageID(page)
		if err != nil {
			return nil, err
		}
		if id == pageID {
			return append(json.RawMessage(nil), page...), nil
		}
	}
	return nil, cmdutil.InvalidInputErrorf("rich content page %s not found", pageID)
}

func mergeRichContentPage(richContent, page json.RawMessage, pageID string) (json.RawMessage, error) {
	id, err := richContentPageID(page)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, cmdutil.InvalidInputErrorf("rich content page JSON must include id %q", pageID)
	}
	if id != pageID {
		return nil, cmdutil.InvalidInputErrorf("rich content page JSON id %q does not match --page %q", id, pageID)
	}

	pages, err := decodeRichContentPages(richContent)
	if err != nil {
		return nil, err
	}
	found := false
	for idx, existing := range pages {
		existingID, err := richContentPageID(existing)
		if err != nil {
			return nil, err
		}
		if existingID == pageID {
			pages[idx] = page
			found = true
			break
		}
	}
	if !found {
		return nil, cmdutil.InvalidInputErrorf("rich content page %s not found", pageID)
	}

	data, err := json.Marshal(pages)
	if err != nil {
		return nil, fmt.Errorf("could not encode merged rich_content JSON: %w", err)
	}
	return data, nil
}

func validateRichContentArray(data []byte) error {
	if !bytes.HasPrefix(data, []byte("[")) {
		return fmt.Errorf("rich content JSON must be an array")
	}
	pages, err := decodeRichContentPages(data)
	if err != nil {
		return fmt.Errorf("rich content JSON must be an array: %w", err)
	}
	for idx, page := range pages {
		if !bytes.HasPrefix(bytes.TrimSpace(page), []byte("{")) {
			return fmt.Errorf("rich content JSON page %d must be an object", idx)
		}
	}
	return nil
}

func decodeRichContentPages(data json.RawMessage) ([]json.RawMessage, error) {
	var pages []json.RawMessage
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

func richContentPageID(page json.RawMessage) (string, error) {
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(page, &parsed); err != nil {
		return "", fmt.Errorf("rich content page must have a string id: %w", err)
	}
	return parsed.ID, nil
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
