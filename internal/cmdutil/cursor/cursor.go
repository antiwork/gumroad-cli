package cursor

import (
	"io"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type Flags struct {
	Cursor string
	Limit  int
}

type Pagination struct {
	Next  string      `json:"next"`
	Limit api.JSONInt `json:"limit"`
}

func AddFlags(cmd *cobra.Command, flags *Flags) {
	cmd.Flags().StringVar(&flags.Cursor, "cursor", "", "Pagination cursor (from a previous response)")
	cmd.Flags().IntVar(&flags.Limit, "limit", 0, "Maximum results per page (default 20)")
}

func Apply(params url.Values, flags Flags) {
	if flags.Cursor != "" {
		params.Set("cursor", flags.Cursor)
	}
	if flags.Limit != 0 {
		params.Set("limit", strconv.Itoa(flags.Limit))
	}
}

func WriteMoreFooter(w io.Writer, p Pagination) error {
	if p.Next == "" {
		return nil
	}
	return output.Writef(w, "\nMore results: --cursor %s\n", p.Next)
}
