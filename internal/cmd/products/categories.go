package products

import (
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type productCategoryListItem struct {
	ID       api.JSONInt  `json:"id"`
	Name     string       `json:"name"`
	Label    string       `json:"label"`
	Path     string       `json:"path"`
	ParentID *api.JSONInt `json:"parent_id"`
}

type productCategoriesResponse struct {
	Success    bool                      `json:"success"`
	Categories []productCategoryListItem `json:"categories"`
}

func newCategoriesCmd() *cobra.Command {
	var search string

	cmd := &cobra.Command{
		Use:   "categories",
		Short: "List product categories",
		Long:  "List product categories. Use the PATH value with products create or products update --category.",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad products categories
  gumroad products categories --search figma
  gumroad products categories --json --jq '.categories[] | {label, path}'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return runProductCategoriesList(opts, search)
		},
	}

	cmd.Flags().StringVar(&search, "search", "", "Filter by label or path")

	return cmd
}

func runProductCategoriesList(opts cmdutil.Options, search string) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	data, err := cmdutil.RunWithTokenData(opts, token, "Fetching product categories...",
		func(client *api.Client) (json.RawMessage, error) {
			return client.Get("/categories", url.Values{})
		})
	if err != nil {
		return err
	}

	resp, err := cmdutil.DecodeJSON[productCategoriesResponse](data)
	if err != nil {
		return err
	}
	resp.Categories = filterProductCategories(resp.Categories, search)

	if opts.UsesJSONOutput() {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}

	return renderProductCategoriesList(opts, resp.Categories, search)
}

func filterProductCategories(categories []productCategoryListItem, search string) []productCategoryListItem {
	query := strings.ToLower(strings.TrimSpace(search))
	if query == "" {
		return categories
	}

	filtered := make([]productCategoryListItem, 0, len(categories))
	for _, category := range categories {
		if strings.Contains(strings.ToLower(category.Label), query) ||
			strings.Contains(strings.ToLower(category.Path), query) {
			filtered = append(filtered, category)
		}
	}
	return filtered
}

func renderProductCategoriesList(opts cmdutil.Options, categories []productCategoryListItem, search string) error {
	if len(categories) == 0 {
		if strings.TrimSpace(search) != "" {
			return cmdutil.PrintInfo(opts, "No product categories found matching "+strconv.Quote(search)+".")
		}
		return cmdutil.PrintInfo(opts, "No product categories found.")
	}

	if opts.PlainOutput {
		rows := make([][]string, 0, len(categories))
		for _, category := range categories {
			rows = append(rows, []string{category.Label, category.Path, categoryIDString(category)})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	style := opts.Style()
	tbl := output.NewStyledTable(style, "LABEL", "PATH", "ID")
	for _, category := range categories {
		tbl.AddRow(category.Label, category.Path, categoryIDString(category))
	}
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		return tbl.Render(w)
	})
}

func categoryIDString(category productCategoryListItem) string {
	return strconv.Itoa(int(category.ID))
}
