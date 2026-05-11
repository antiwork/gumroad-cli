package products

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type listResponse struct {
	Products   []product         `json:"products"`
	Pagination productPagination `json:"pagination"`
}

type productPagination struct {
	Count int  `json:"count"`
	Items int  `json:"items"`
	Page  int  `json:"page"`
	Pages int  `json:"pages"`
	Prev  *int `json:"prev"`
	Next  *int `json:"next"`
	Last  int  `json:"last"`
}

func newListCmd() *cobra.Command {
	var (
		email      string
		externalID string
		page       int
		perPage    int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List products owned by a seller",
		Long: `List products owned by a seller, including soft-deleted products and the
files attached to each product. Soft-deleted rows are returned with a (deleted)
indicator instead of being filtered out so risk reviewers can see tombstones.

Identify the seller with --email or --external-id. Use --external-id when the
seller is suspended-for-fraud, soft-deleted, or has had their email rotated,
since those are the cases where the email path can't resolve them.`,
		Example: `  gumroad admin products list --email seller@example.com
  gumroad admin products list --external-id 2245593582708
  gumroad admin products list --email seller@example.com --page 2 --per-page 25
  gumroad admin products list --email seller@example.com --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" && externalID == "" {
				return cmdutil.UsageErrorf(c, "missing required flag: --email or --external-id")
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "page", page); err != nil {
				return err
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "per-page", perPage); err != nil {
				return err
			}

			params := url.Values{}
			if email != "" {
				params.Set("email", email)
			}
			if externalID != "" {
				params.Set("external_id", externalID)
			}
			if c.Flags().Changed("page") {
				params.Set("page", strconv.Itoa(page))
			}
			if c.Flags().Changed("per-page") {
				params.Set("per_page", strconv.Itoa(perPage))
			}

			return admincmd.RunGetDecoded[listResponse](opts, "Fetching products...", "/products", params, func(resp listResponse) error {
				return renderList(opts, sellerSubject(email, externalID), resp)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Seller email (required unless --external-id is set)")
	cmd.Flags().StringVar(&externalID, "external-id", "", "Seller external id (required unless --email is set)")
	cmd.Flags().IntVar(&page, "page", 0, "Page number (default 1)")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Results per page (default 10, max 100)")

	return cmd
}

func sellerSubject(email, externalID string) string {
	if externalID != "" {
		return "external_id " + externalID
	}
	return email
}

func renderList(opts cmdutil.Options, subject string, resp listResponse) error {
	if opts.PlainOutput {
		return writeListPlain(opts.Out(), resp.Products)
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.Products) == 0 {
			return output.Writef(w, "%s", buildEmptyListBody(subject, resp.Pagination))
		}
		if err := output.Writef(w, "%s", buildListHeader(style, subject)); err != nil {
			return err
		}
		if err := writeListTable(w, style, resp.Products); err != nil {
			return err
		}
		if footer := paginationFooter(resp.Pagination); footer != "" {
			return output.Writef(w, "\n%s\n", footer)
		}
		return nil
	})
}

func buildListHeader(style output.Styler, subject string) string {
	var b strings.Builder
	fmt.Fprintln(&b, style.Bold(fmt.Sprintf("Products for %s", subject)))
	fmt.Fprintln(&b)
	return b.String()
}

func buildEmptyListBody(subject string, p productPagination) string {
	var b strings.Builder
	fmt.Fprintf(&b, "No products found for %s.\n", subject)
	if footer := paginationFooter(p); footer != "" {
		fmt.Fprintln(&b, footer)
	}
	return b.String()
}

func writeListPlain(w io.Writer, products []product) error {
	rows := make([][]string, 0, len(products))
	for _, p := range products {
		rows = append(rows, []string{
			p.ID,
			productNameWithDeleted(p),
			formatPrice(p.PriceCents, p.CurrencyCode),
			strconv.Itoa(len(p.Files)),
			productStatusLabel(p),
			p.CreatedAt,
		})
	}
	return output.PrintPlain(w, rows)
}

func writeListTable(w io.Writer, style output.Styler, products []product) error {
	tbl := output.NewStyledTable(style, "ID", "NAME", "PRICE", "FILES", "STATUS", "CREATED")
	for _, p := range products {
		tbl.AddRow(
			p.ID,
			productNameWithDeleted(p),
			formatPrice(p.PriceCents, p.CurrencyCode),
			strconv.Itoa(len(p.Files)),
			productStatusLabel(p),
			p.CreatedAt,
		)
	}
	return tbl.Render(w)
}

func paginationFooter(p productPagination) string {
	if p.Pages == 0 && p.Count == 0 && p.Page == 0 {
		return ""
	}
	pages := p.Pages
	if pages == 0 {
		pages = 1
	}
	page := p.Page
	if page == 0 {
		page = 1
	}
	return fmt.Sprintf("Page %d of %d (%d total)", page, pages, p.Count)
}
