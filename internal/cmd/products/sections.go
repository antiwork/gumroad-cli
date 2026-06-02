package products

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type sectionItem struct {
	ID                 string   `json:"id"`
	Type               string   `json:"type"`
	Header             string   `json:"header"`
	HideHeader         bool     `json:"hide_header"`
	ShownProducts      []string `json:"shown_products,omitempty"`
	DefaultProductSort string   `json:"default_product_sort,omitempty"`
	ShowFilters        bool     `json:"show_filters,omitempty"`
	AddNewProducts     bool     `json:"add_new_products,omitempty"`
	FeaturedProduct    string   `json:"featured_product,omitempty"`
}

type productWithSections struct {
	Sections         []sectionItem `json:"sections"`
	MainSectionIndex int           `json:"main_section_index"`
}

type productSectionsResponse struct {
	Product          *productWithSections `json:"product,omitempty"`
	Sections         []sectionItem        `json:"sections,omitempty"`
	MainSectionIndex int                  `json:"main_section_index,omitempty"`
}

type rawProductWithSections struct {
	Sections         json.RawMessage `json:"sections"`
	MainSectionIndex *int            `json:"main_section_index"`
}

type rawProductSectionsResponse struct {
	Product          *rawProductWithSections `json:"product,omitempty"`
	Sections         json.RawMessage         `json:"sections,omitempty"`
	MainSectionIndex *int                    `json:"main_section_index,omitempty"`
}

type sectionsListOutput struct {
	Success          bool            `json:"success"`
	Sections         json.RawMessage `json:"sections"`
	MainSectionIndex int             `json:"main_section_index"`
}

func newSectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sections",
		Short: "Manage product page sections",
		Long: "Manage product page sections.\n\n" +
			"List a product's page sections in display order. Section writes are handled by dedicated verbs as the API exposes them.",
		Example: `  gumroad products sections list <product_id>`,
	}

	cmd.AddCommand(newSectionsListCmd())
	return cmd
}

func newSectionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list <product_id>",
		Short:   "List product page sections",
		Args:    cmdutil.ExactArgs(1),
		Example: `  gumroad products sections list <product_id>`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			requestOpts := opts
			if opts.UsesJSONOutput() {
				requestOpts.JSONOutput = false
				requestOpts.JQExpr = ""
			}

			return cmdutil.RunRequest(requestOpts, "Fetching sections...", http.MethodGet, cmdutil.JoinPath("products", args[0]), url.Values{}, func(data json.RawMessage) error {
				return renderSectionsListData(opts, data)
			})
		},
	}
}

func (r productSectionsResponse) product() productWithSections {
	if r.Product != nil {
		return *r.Product
	}
	return productWithSections{
		Sections:         r.Sections,
		MainSectionIndex: r.MainSectionIndex,
	}
}

func (r rawProductSectionsResponse) product() rawProductWithSections {
	if r.Product != nil {
		return *r.Product
	}
	return rawProductWithSections{
		Sections:         r.Sections,
		MainSectionIndex: r.MainSectionIndex,
	}
}

func renderSectionsListData(opts cmdutil.Options, data json.RawMessage) error {
	rawResp, err := cmdutil.DecodeJSON[rawProductSectionsResponse](data)
	if err != nil {
		return err
	}
	rawProduct := rawResp.product()
	if err := validateSectionsResponse(rawProduct); err != nil {
		return err
	}

	if opts.UsesJSONOutput() {
		return renderSectionsListJSON(opts, rawProduct)
	}

	resp, err := cmdutil.DecodeJSON[productSectionsResponse](data)
	if err != nil {
		return err
	}
	return renderSectionsList(opts, resp.product())
}

func renderSectionsListJSON(opts cmdutil.Options, product rawProductWithSections) error {
	sections := nonEmptyRawSections(product.Sections)
	data, err := json.Marshal(sectionsListOutput{
		Success:          true,
		Sections:         sections,
		MainSectionIndex: *product.MainSectionIndex,
	})
	if err != nil {
		return fmt.Errorf("could not encode JSON output: %w", err)
	}
	return output.PrintJSON(opts.Out(), data, opts.JQExpr)
}

func validateSectionsResponse(product rawProductWithSections) error {
	sections := bytes.TrimSpace(product.Sections)
	if len(sections) == 0 || product.MainSectionIndex == nil {
		return fmt.Errorf("product sections are not available in this API response; deploy the product sections API response before using `gumroad products sections list`")
	}
	if !bytes.HasPrefix(sections, []byte("[")) {
		return fmt.Errorf("product sections response is invalid: sections must be an array")
	}
	var decodedSections []json.RawMessage
	if err := json.Unmarshal(sections, &decodedSections); err != nil {
		return fmt.Errorf("product sections response is invalid: sections must be an array")
	}
	return nil
}

func renderSectionsList(opts cmdutil.Options, product productWithSections) error {
	product.Sections = nonNilSections(product.Sections)

	if len(product.Sections) == 0 {
		return cmdutil.PrintInfo(opts, "No sections found.")
	}

	if opts.PlainOutput {
		var rows [][]string
		for idx, section := range product.Sections {
			main := ""
			if idx == product.MainSectionIndex {
				main = "main"
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", idx),
				section.ID,
				section.Type,
				section.Header,
				sectionDetails(section),
				main,
			})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	style := opts.Style()
	tbl := output.NewStyledTable(style, "#", "ID", "TYPE", "HEADER", "DETAILS")
	for idx, section := range product.Sections {
		tbl.AddRow(
			formatSectionIndex(style, idx, idx == product.MainSectionIndex),
			section.ID,
			section.Type,
			formatSectionHeader(section.Header),
			sectionDetails(section),
		)
	}
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		return tbl.Render(w)
	})
}

func nonNilSections(sections []sectionItem) []sectionItem {
	if sections == nil {
		return []sectionItem{}
	}
	return sections
}

func nonEmptyRawSections(sections json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(sections)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage("[]")
	}
	return sections
}

func formatSectionIndex(style output.Styler, idx int, main bool) string {
	label := fmt.Sprintf("%d", idx)
	if main {
		return style.Green(label + " *")
	}
	return label
}

func formatSectionHeader(header string) string {
	if header == "" {
		return "-"
	}
	return header
}

func sectionDetails(section sectionItem) string {
	if isFeaturedSection(section) {
		if section.FeaturedProduct == "" {
			return "featured=-"
		}
		return "featured=" + section.FeaturedProduct
	}

	if isProductsSection(section) {
		parts := []string{fmt.Sprintf("products=%d", len(section.ShownProducts))}
		if section.DefaultProductSort != "" {
			parts = append(parts, "sort="+section.DefaultProductSort)
		}
		return strings.Join(parts, " ")
	}

	return "-"
}

func isFeaturedSection(section sectionItem) bool {
	return strings.Contains(normalizeSectionType(section.Type), "featured") || section.FeaturedProduct != ""
}

func isProductsSection(section sectionItem) bool {
	kind := normalizeSectionType(section.Type)
	return kind == "products" ||
		kind == "product_list" ||
		strings.Contains(kind, "products") ||
		len(section.ShownProducts) > 0 ||
		section.DefaultProductSort != "" ||
		section.ShowFilters ||
		section.AddNewProducts
}

func normalizeSectionType(sectionType string) string {
	return strings.ToLower(strings.ReplaceAll(sectionType, "-", "_"))
}
