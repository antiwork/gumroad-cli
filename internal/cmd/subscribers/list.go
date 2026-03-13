package subscribers

import (
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type subscriberListItem struct {
	ID        string `json:"id"`
	Email     string `json:"email_address"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type subscribersListResponse struct {
	Success     bool                 `json:"success"`
	Subscribers []subscriberListItem `json:"subscribers"`
	NextPageKey string               `json:"next_page_key,omitempty"`
}

func newListCmd() *cobra.Command {
	var product, email, pageKey string
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List subscribers for a product",
		Args:  cmdutil.ExactArgs(0),
		Long:  "List subscribers for a product. Results are paginated by default (one page at a time).",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}

			params := url.Values{}
			params.Set("paginated", "true")
			if email != "" {
				params.Set("email", email)
			}
			if pageKey != "" {
				params.Set("page_key", pageKey)
			}
			path := cmdutil.JoinPath("products", product, "subscribers")
			if all {
				return streamSubscribersListAll(opts, path, params)
			}

			return cmdutil.RunRequestDecoded[subscribersListResponse](opts, "Fetching subscribers...", "GET", path, params, func(resp subscribersListResponse) error {
				return renderSubscribersList(opts, resp, product, email)
			})
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&email, "email", "", "Filter by email")
	cmd.Flags().StringVar(&pageKey, "page-key", "", "Pagination cursor")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages")

	return cmd
}

func renderSubscribersList(opts cmdutil.Options, resp subscribersListResponse, product, email string) error {
	if len(resp.Subscribers) == 0 {
		return renderEmptySubscribersList(opts, product, email, resp.NextPageKey)
	}

	if opts.PlainOutput {
		return writeSubscribersPlain(opts.Out(), resp.Subscribers)
	}

	style := opts.Style()
	hint := subscriberPaginationHint(product, email, resp.NextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writeSubscribersTable(w, style, resp.Subscribers); err != nil {
			return err
		}
		if resp.NextPageKey != "" && !opts.Quiet {
			return output.Writeln(w, style.Dim("\nMore results available: "+hint))
		}
		return nil
	})
}

func streamSubscribersListAll(opts cmdutil.Options, path string, params url.Values) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	sp := output.NewSpinnerTo("Fetching subscribers...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, token)
	style := opts.Style()
	walkPages := func(visit cmdutil.PageVisitor[subscribersListResponse]) error {
		return walkSubscriberPages(opts, client, path, params, visit)
	}

	return cmdutil.StreamPaginatedPages(opts, cmdutil.PaginatedPageOutputConfig[subscribersListResponse]{
		JSONKey:      "subscribers",
		EmptyMessage: "No subscribers found.",
		Walk:         walkPages,
		HasItems:     hasSubscribers,
		WriteItems:   writeSubscriberItems,
		WritePlainPage: func(w io.Writer, page subscribersListResponse) error {
			return writeSubscribersPlain(w, page.Subscribers)
		},
		WriteTablePage: func(w io.Writer, page subscribersListResponse) error {
			return writeSubscribersTable(w, style, page.Subscribers)
		},
	})
}

func walkSubscriberPages(opts cmdutil.Options, client *api.Client, path string, params url.Values, visit cmdutil.PageVisitor[subscribersListResponse]) error {
	return cmdutil.WalkPagesWithDelay[subscribersListResponse](opts.Context, opts.PageDelay, client, path, params, func(page subscribersListResponse) string {
		return page.NextPageKey
	}, visit)
}

func hasSubscribers(page subscribersListResponse) bool {
	return len(page.Subscribers) > 0
}

func writeSubscriberItems(page subscribersListResponse, writeItem func(any) error) error {
	for _, subscriber := range page.Subscribers {
		if err := writeItem(subscriber); err != nil {
			return err
		}
	}
	return nil
}

func writeSubscribersPlain(w io.Writer, subscribers []subscriberListItem) error {
	var rows [][]string
	for _, s := range subscribers {
		rows = append(rows, []string{s.ID, s.Email, s.Status, s.CreatedAt})
	}
	return output.PrintPlain(w, rows)
}

func writeSubscribersTable(w io.Writer, style output.Styler, subscribers []subscriberListItem) error {
	tbl := output.NewStyledTable(style, "ID", "EMAIL", "STATUS", "SUBSCRIBED")
	for _, s := range subscribers {
		status := s.Status
		switch s.Status {
		case "alive":
			status = style.Green(s.Status)
		case "cancelled":
			status = style.Red(s.Status)
		}
		tbl.AddRow(s.ID, s.Email, status, s.CreatedAt)
	}
	return tbl.Render(w)
}

func renderEmptySubscribersList(opts cmdutil.Options, product, email, nextPageKey string) error {
	if nextPageKey == "" || opts.PlainOutput || opts.Quiet {
		return cmdutil.PrintInfo(opts, "No subscribers found.")
	}

	style := opts.Style()
	hint := subscriberPaginationHint(product, email, nextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, "No subscribers found on this page."); err != nil {
			return err
		}
		return output.Writeln(w, style.Dim("More results available: "+hint))
	})
}

func subscriberPaginationHint(product, email, nextPageKey string) string {
	return cmdutil.ReplayCommand("gumroad subscribers list",
		cmdutil.CommandArg{Flag: "--product", Value: product},
		cmdutil.CommandArg{Flag: "--email", Value: email},
		cmdutil.CommandArg{Flag: "--page-key", Value: nextPageKey},
	)
}
