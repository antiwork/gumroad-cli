package payouts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type payoutListItem struct {
	ID                  string      `json:"id"`
	PayoutPeriod        string      `json:"payout_period"`
	AmountCents         api.JSONInt `json:"amount_cents"`
	FormattedAmount     string      `json:"formatted_amount"`
	IsUpcoming          bool        `json:"is_upcoming"`
	DisplayPayoutPeriod string      `json:"display_payout_period"`
}

type payoutsListResponse struct {
	Success     bool             `json:"success"`
	Payouts     []payoutListItem `json:"payouts"`
	NextPageKey string           `json:"next_page_key,omitempty"`
}

func newListCmd() *cobra.Command {
	var before, after, pageKey string
	var noUpcoming, all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List payouts",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := cmdutil.RequireDateFlag(c, "before", before); err != nil {
				return err
			}
			if err := cmdutil.RequireDateFlag(c, "after", after); err != nil {
				return err
			}

			params := url.Values{}
			if before != "" {
				params.Set("before", before)
			}
			if after != "" {
				params.Set("after", after)
			}
			if pageKey != "" {
				params.Set("page_key", pageKey)
			}
			if all {
				return streamPayoutsListAll(opts, params, noUpcoming)
			}

			return cmdutil.RunDecoded[payoutsListResponse](opts, "Fetching payouts...", func(client *api.Client) (json.RawMessage, error) {
				return fetchPayoutsListData(client, params, noUpcoming)
			}, func(resp payoutsListResponse) error {
				return renderPayoutsList(opts, resp, before, after, noUpcoming)
			})
		},
	}

	cmd.Flags().StringVar(&before, "before", "", "Filter payouts before date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&after, "after", "", "Filter payouts after date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&pageKey, "page-key", "", "Pagination cursor")
	cmd.Flags().BoolVar(&noUpcoming, "no-upcoming", false, "Exclude upcoming payouts")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages")
	cmd.MarkFlagsMutuallyExclusive("all", "page-key")

	return cmd
}

func renderPayoutsList(opts cmdutil.Options, resp payoutsListResponse, before, after string, noUpcoming bool) error {
	if len(resp.Payouts) == 0 {
		return renderEmptyPayoutsList(opts, before, after, noUpcoming, resp.NextPageKey)
	}

	if opts.PlainOutput {
		return writePayoutsPlain(opts.Out(), resp.Payouts)
	}

	style := opts.Style()
	hint := payoutPaginationHint(before, after, noUpcoming, resp.NextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := writePayoutsTable(w, style, resp.Payouts); err != nil {
			return err
		}
		if resp.NextPageKey != "" && !opts.Quiet {
			return output.Writeln(w, style.Dim("\nMore results available: "+hint))
		}
		return nil
	})
}

func fetchPayoutsListData(client *api.Client, params url.Values, noUpcoming bool) (json.RawMessage, error) {
	data, err := client.Get("/payouts", params)
	if err != nil {
		return nil, err
	}
	if !noUpcoming {
		return data, nil
	}

	return filterPayoutsRaw(data)
}

// filterPayoutsRaw removes upcoming payouts from raw JSON without losing
// unknown fields. It works with map[string]json.RawMessage so that
// top-level and per-item fields the CLI doesn't know about are preserved.
func filterPayoutsRaw(data json.RawMessage) (json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	raw, ok := envelope["payouts"]
	if !ok || string(raw) == "null" {
		return data, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("could not parse payouts array: %w", err)
	}

	filtered := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		var fields struct {
			IsUpcoming bool `json:"is_upcoming"`
		}
		if err := json.Unmarshal(item, &fields); err != nil {
			return nil, fmt.Errorf("could not parse payout item: %w", err)
		}
		if !fields.IsUpcoming {
			filtered = append(filtered, item)
		}
	}

	rawFiltered, err := json.Marshal(filtered)
	if err != nil {
		return nil, fmt.Errorf("could not encode filtered payouts: %w", err)
	}
	envelope["payouts"] = rawFiltered

	result, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("could not encode response: %w", err)
	}
	return result, nil
}

func streamPayoutsListAll(opts cmdutil.Options, params url.Values, noUpcoming bool) error {
	token, err := config.Token()
	if err != nil {
		return err
	}

	sp := output.NewSpinnerTo("Fetching payouts...", opts.Err())
	if cmdutil.ShouldShowSpinner(opts) {
		sp.Start()
	}
	defer sp.Stop()

	client := cmdutil.NewAPIClient(opts, token)
	style := opts.Style()
	walkPages := func(visit cmdutil.PageVisitor[payoutsListResponse]) error {
		return walkPayoutPages(opts, client, params, visit)
	}
	visiblePayouts := func(page payoutsListResponse) []payoutListItem {
		return filterPayouts(page.Payouts, noUpcoming)
	}

	return cmdutil.StreamPaginatedPages(opts, cmdutil.PaginatedPageOutputConfig[payoutsListResponse]{
		JSONKey:      "payouts",
		EmptyMessage: "No payouts found.",
		Walk:         walkPages,
		HasItems: func(page payoutsListResponse) bool {
			return len(visiblePayouts(page)) > 0
		},
		WriteItems: func(page payoutsListResponse, writeItem func(any) error) error {
			return writePayoutItems(visiblePayouts(page), writeItem)
		},
		WritePlainPage: func(w io.Writer, page payoutsListResponse) error {
			return writePayoutsPlain(w, visiblePayouts(page))
		},
		WriteTablePage: func(w io.Writer, page payoutsListResponse) error {
			return writePayoutsTable(w, style, visiblePayouts(page))
		},
	})
}

func walkPayoutPages(opts cmdutil.Options, client *api.Client, params url.Values, visit cmdutil.PageVisitor[payoutsListResponse]) error {
	return cmdutil.WalkPagesWithDelay[payoutsListResponse](opts.Context, opts.PageDelay, client, "/payouts", params, func(page payoutsListResponse) string {
		return page.NextPageKey
	}, visit)
}

func writePayoutItems(payouts []payoutListItem, writeItem func(any) error) error {
	for _, payout := range payouts {
		if err := writeItem(payout); err != nil {
			return err
		}
	}
	return nil
}

func writePayoutsPlain(w io.Writer, payouts []payoutListItem) error {
	var rows [][]string
	for _, p := range payouts {
		rows = append(rows, []string{p.ID, p.DisplayPayoutPeriod, p.FormattedAmount})
	}
	return output.PrintPlain(w, rows)
}

func writePayoutsTable(w io.Writer, style output.Styler, payouts []payoutListItem) error {
	tbl := output.NewStyledTable(style, "ID", "PERIOD", "AMOUNT")
	for _, p := range payouts {
		period := p.DisplayPayoutPeriod
		if p.IsUpcoming {
			period += " " + style.Yellow("(upcoming)")
		}
		tbl.AddRow(p.ID, period, p.FormattedAmount)
	}
	return tbl.Render(w)
}

func filterPayouts(payouts []payoutListItem, noUpcoming bool) []payoutListItem {
	if !noUpcoming {
		return payouts
	}

	filtered := make([]payoutListItem, 0, len(payouts))
	for _, payout := range payouts {
		if !payout.IsUpcoming {
			filtered = append(filtered, payout)
		}
	}
	return filtered
}

func renderEmptyPayoutsList(opts cmdutil.Options, before, after string, noUpcoming bool, nextPageKey string) error {
	if nextPageKey == "" || opts.PlainOutput || opts.Quiet {
		return cmdutil.PrintInfo(opts, "No payouts found.")
	}

	style := opts.Style()
	hint := payoutPaginationHint(before, after, noUpcoming, nextPageKey)
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if err := output.Writeln(w, "No payouts found on this page."); err != nil {
			return err
		}
		return output.Writeln(w, style.Dim("More results available: "+hint))
	})
}

func payoutPaginationHint(before, after string, noUpcoming bool, nextPageKey string) string {
	return cmdutil.ReplayCommand("gumroad payouts list",
		cmdutil.CommandArg{Flag: "--before", Value: before},
		cmdutil.CommandArg{Flag: "--after", Value: after},
		cmdutil.CommandArg{Flag: "--no-upcoming", Boolean: noUpcoming},
		cmdutil.CommandArg{Flag: "--page-key", Value: nextPageKey},
	)
}
