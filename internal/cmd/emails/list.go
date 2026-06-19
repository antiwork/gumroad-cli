package emails

import (
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type emailListResponse struct {
	Success     bool              `json:"success"`
	Emails      []emailRecord     `json:"emails"`
	RawEmails   []json.RawMessage `json:"-"`
	NextPageKey string            `json:"next_page_key,omitempty"`
	NextPageURL string            `json:"next_page_url,omitempty"`
}

func (resp *emailListResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Success     bool              `json:"success"`
		Emails      []json.RawMessage `json:"emails"`
		NextPageKey string            `json:"next_page_key,omitempty"`
		NextPageURL string            `json:"next_page_url,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	emails := make([]emailRecord, 0, len(raw.Emails))
	for _, data := range raw.Emails {
		var item emailRecord
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		emails = append(emails, item)
	}

	resp.Success = raw.Success
	resp.Emails = emails
	resp.RawEmails = raw.Emails
	resp.NextPageKey = raw.NextPageKey
	resp.NextPageURL = raw.NextPageURL
	return nil
}

func newListCmd() *cobra.Command {
	var state, pageKey string
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audience emails",
		Long:  "List audience emails by draft, scheduled, or published state.",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad emails list
  gumroad emails list --state draft
  gumroad emails list --state published --all
  gumroad emails list --json --jq '.emails[0].id'`,
		RunE: func(c *cobra.Command, args []string) error {
			if state != "" && !emailValidValue(state, emailValidStateValues()) {
				return cmdutil.UsageErrorf(c, "--state must be one of: %s", strings.Join(emailValidStateValues(), ", "))
			}

			params := url.Values{}
			if state != "" {
				params.Set("type", state)
			}
			if pageKey != "" {
				params.Set("page_key", pageKey)
			}

			opts := cmdutil.OptionsFrom(c)
			if all {
				return streamEmailListAll(opts, params)
			}

			return cmdutil.RunRequestDecoded[emailListResponse](opts, "Fetching emails...", "GET", cmdutil.JoinPath("emails"), params, func(resp emailListResponse) error {
				return renderEmailList(opts, resp, state)
			})
		},
	}

	cmd.Flags().StringVar(&state, "state", "", "Filter by state: published, scheduled, draft")
	cmd.Flags().StringVar(&pageKey, "page-key", "", "Pagination cursor")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages")
	cmd.MarkFlagsMutuallyExclusive("all", "page-key")

	return cmd
}

func renderEmailList(opts cmdutil.Options, resp emailListResponse, state string) error {
	if len(resp.Emails) == 0 {
		return renderEmptyEmailList(opts, state, resp.NextPageKey)
	}

	if opts.PlainOutput {
		return writeEmailPlain(opts.Out(), resp.Emails)
	}

	style := opts.Style()
	hint := emailPaginationHint(state, resp.NextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writeEmailTable(w, style, resp.Emails); err != nil {
			return err
		}
		if resp.NextPageKey != "" && !opts.Quiet {
			return output.Writeln(w, style.Dim("\nMore results available: "+hint))
		}
		return nil
	})
}

func streamEmailListAll(opts cmdutil.Options, params url.Values) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	sp := output.NewSpinnerTo("Fetching emails...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, token)
	style := opts.Style()
	walkPages := func(visit cmdutil.PageVisitor[emailListResponse]) error {
		return walkEmailPages(opts, client, params, visit)
	}

	return cmdutil.StreamPaginatedPages(opts, cmdutil.PaginatedPageOutputConfig[emailListResponse]{
		JSONKey:      "emails",
		EmptyMessage: "No emails found.",
		Walk:         walkPages,
		HasItems:     hasEmails,
		WriteItems:   writeEmailItems,
		WritePlainPage: func(w io.Writer, page emailListResponse) error {
			return writeEmailPlain(w, page.Emails)
		},
		WriteTablePage: func(w io.Writer, page emailListResponse) error {
			return writeEmailTable(w, style, page.Emails)
		},
	})
}

func walkEmailPages(opts cmdutil.Options, client *api.Client, params url.Values, visit cmdutil.PageVisitor[emailListResponse]) error {
	return cmdutil.WalkPagesWithDelay[emailListResponse](opts.Context, opts.PageDelay, client, cmdutil.JoinPath("emails"), params, func(page emailListResponse) string {
		return page.NextPageKey
	}, visit)
}

func hasEmails(page emailListResponse) bool {
	return len(page.Emails) > 0
}

func writeEmailItems(page emailListResponse, writeItem func(any) error) error {
	if len(page.RawEmails) > 0 {
		for _, item := range page.RawEmails {
			if err := writeItem(item); err != nil {
				return err
			}
		}
		return nil
	}

	for _, item := range page.Emails {
		if err := writeItem(item); err != nil {
			return err
		}
	}
	return nil
}

func writeEmailPlain(w io.Writer, items []emailRecord) error {
	var rows [][]string
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.Subject, item.State, emailAudienceLabel(item), item.ProductID, emailDisplayDate(item)})
	}
	return output.PrintPlain(w, rows)
}

func writeEmailTable(w io.Writer, style output.Styler, items []emailRecord) error {
	tbl := output.NewStyledTable(style, "ID", "SUBJECT", "STATE", "AUDIENCE", "PRODUCT", "PUBLISHED/SCHEDULED AT")
	for _, item := range items {
		tbl.AddRow(item.ID, item.Subject, item.State, emailAudienceLabel(item), item.ProductID, emailDisplayDate(item))
	}
	return tbl.Render(w)
}

func renderEmptyEmailList(opts cmdutil.Options, state, nextPageKey string) error {
	if nextPageKey == "" || opts.PlainOutput || opts.Quiet {
		return cmdutil.PrintInfo(opts, "No emails found.")
	}

	style := opts.Style()
	hint := emailPaginationHint(state, nextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, "No emails found on this page."); err != nil {
			return err
		}
		return output.Writeln(w, style.Dim("More results available: "+hint))
	})
}

func emailPaginationHint(state, nextPageKey string) string {
	return cmdutil.ReplayCommand("gumroad emails list",
		cmdutil.CommandArg{Flag: "--state", Value: state},
		cmdutil.CommandArg{Flag: "--page-key", Value: nextPageKey},
	)
}
