package media

import (
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

type mediaListResponse struct {
	Media []mediaItem `json:"media"`
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List public media library files",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[mediaListResponse](opts, "Fetching media...", "GET", "/media", url.Values{}, func(resp mediaListResponse) error {
				if len(resp.Media) == 0 {
					return cmdutil.PrintInfo(opts, "No media library files found.")
				}

				if opts.PlainOutput {
					var rows [][]string
					for _, m := range resp.Media {
						rows = append(rows, []string{m.ID, m.Name, uploadui.HumanBytes(m.FileSize), m.URL})
					}
					return output.PrintPlain(opts.Out(), rows)
				}

				style := opts.Style()
				tbl := output.NewStyledTable(style, "ID", "NAME", "SIZE", "URL")
				for _, m := range resp.Media {
					tbl.AddRow(m.ID, m.Name, uploadui.HumanBytes(m.FileSize), m.URL)
				}
				return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
					return tbl.Render(w)
				})
			})
		},
	}
}
