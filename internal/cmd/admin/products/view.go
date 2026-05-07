package products

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type product struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	PriceCents   api.JSONInt   `json:"price_cents"`
	CurrencyCode string        `json:"currency_code"`
	Permalink    string        `json:"permalink"`
	LongURL      string        `json:"long_url"`
	PreviewURL   string        `json:"preview_url"`
	CreatedAt    string        `json:"created_at"`
	DeletedAt    string        `json:"deleted_at"`
	Alive        bool          `json:"alive"`
	IsAdult      bool          `json:"is_adult"`
	Seller       productSeller `json:"seller"`
	Files        []productFile `json:"files"`
}

type productSeller struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type productFile struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	FileName    string      `json:"file_name"`
	Extension   string      `json:"extension"`
	Filegroup   string      `json:"filegroup"`
	FileSize    api.JSONInt `json:"file_size"`
	CreatedAt   string      `json:"created_at"`
	DeletedAt   string      `json:"deleted_at"`
}

type viewResponse struct {
	Product product `json:"product"`
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <product-id>",
		Short: "View an admin product record",
		Example: `  gumroad admin products view abc123
  gumroad admin products view abc123 --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath("products", args[0])
			return admincmd.RunGetDecoded[viewResponse](opts, "Fetching product...", path, url.Values{}, func(resp viewResponse) error {
				return renderProduct(opts, resp.Product)
			})
		},
	}
}

func renderProduct(opts cmdutil.Options, p product) error {
	if opts.PlainOutput {
		return writeProductPlain(opts.Out(), p)
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writef(w, "%s", buildProductHeader(style, p)); err != nil {
			return err
		}
		return writeFilesSection(w, style, p.Files)
	})
}

func buildProductHeader(style output.Styler, p product) string {
	var b strings.Builder

	headline := productNameWithDeleted(p)
	if headline == "" {
		headline = p.ID
	}
	fmt.Fprintln(&b, style.Bold(headline))
	fmt.Fprintf(&b, "ID: %s\n", p.ID)
	if p.Permalink != "" && p.Permalink != p.ID {
		fmt.Fprintf(&b, "Permalink: %s\n", p.Permalink)
	}
	if p.LongURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", p.LongURL)
	}
	if label := sellerLabel(p.Seller); label != "" {
		fmt.Fprintf(&b, "Seller: %s\n", label)
	}
	fmt.Fprintf(&b, "Price: %s\n", formatPrice(p.PriceCents, p.CurrencyCode))
	fmt.Fprintf(&b, "Status: %s\n", productStatusLabel(p))
	if p.IsAdult {
		fmt.Fprintln(&b, "Adult: true")
	}
	if p.CreatedAt != "" {
		fmt.Fprintf(&b, "Created: %s\n", p.CreatedAt)
	}
	if p.DeletedAt != "" {
		fmt.Fprintf(&b, "Deleted: %s\n", p.DeletedAt)
	}
	if p.Description != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, style.Bold("Description:"))
		fmt.Fprintln(&b, p.Description)
	}
	fmt.Fprintln(&b)

	return b.String()
}

func writeProductPlain(w io.Writer, p product) error {
	return output.PrintPlain(w, [][]string{{
		p.ID,
		productNameWithDeleted(p),
		formatPrice(p.PriceCents, p.CurrencyCode),
		productStatusLabel(p),
		p.Seller.Email,
		strconv.Itoa(len(p.Files)),
		p.CreatedAt,
		p.LongURL,
	}})
}

func writeFilesSection(w io.Writer, style output.Styler, files []productFile) error {
	if len(files) == 0 {
		return output.Writef(w, "%s none\n", style.Bold("Files:"))
	}
	header := style.Bold(fmt.Sprintf("Files (%d):", len(files))) + "\n"
	if err := output.Writef(w, "%s", header); err != nil {
		return err
	}
	tbl := output.NewStyledTable(style, "DISPLAY NAME", "FILE NAME", "EXT", "GROUP", "SIZE", "CREATED")
	for _, f := range files {
		tbl.AddRow(
			fileDisplayNameWithDeleted(f),
			f.FileName,
			f.Extension,
			f.Filegroup,
			formatFileSize(int(f.FileSize)),
			f.CreatedAt,
		)
	}
	return tbl.Render(w)
}

func fileDisplayNameWithDeleted(f productFile) string {
	name := f.DisplayName
	if name == "" {
		name = f.FileName
	}
	if f.DeletedAt != "" {
		if name == "" {
			return "(deleted)"
		}
		return name + " (deleted)"
	}
	return name
}

func sellerLabel(s productSeller) string {
	switch {
	case s.Email != "" && s.ID != "":
		return s.Email + " (" + s.ID + ")"
	case s.Email != "":
		return s.Email
	default:
		return s.ID
	}
}

func productStatusLabel(p product) string {
	if p.DeletedAt != "" {
		return "deleted"
	}
	if p.Alive {
		return "alive"
	}
	return "unpublished"
}

func productNameWithDeleted(p product) string {
	if p.Name == "" {
		if p.DeletedAt != "" {
			return "(deleted)"
		}
		return ""
	}
	if p.DeletedAt != "" {
		return p.Name + " (deleted)"
	}
	return p.Name
}

func formatPrice(cents api.JSONInt, currency string) string {
	formatted := cmdutil.FormatMoney(int(cents), currency)
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "" {
		return formatted
	}
	return formatted + " " + code
}

const bytesPerKiB = 1024

func formatFileSize(bytes int) string {
	if bytes < 0 {
		bytes = 0
	}
	if bytes < bytesPerKiB {
		return fmt.Sprintf("%d B", bytes)
	}
	size := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	var unit string
	for _, u := range units {
		size /= bytesPerKiB
		unit = u
		if size < bytesPerKiB {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", size, unit)
}
