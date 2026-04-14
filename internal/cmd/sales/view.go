package sales

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View a sale",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequest(opts, "Fetching sale...", "GET", cmdutil.JoinPath("sales", args[0]), url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					Sale struct {
						ID             string      `json:"id"`
						Email          string      `json:"email"`
						ProductName    string      `json:"product_name"`
						FormattedTotal string      `json:"formatted_total_price"`
						CreatedAt      string      `json:"created_at"`
						Refunded       bool        `json:"refunded"`
						Shipped        bool        `json:"shipped"`
						OrderID        api.JSONInt `json:"order_id"`
					} `json:"sale"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				s := resp.Sale
				style := opts.Style()

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{s.ID, s.Email, s.ProductName, s.FormattedTotal, s.CreatedAt, fmt.Sprintf("refunded=%v", s.Refunded)},
					})
				}

				if err := output.Writef(opts.Out(), "%s  %s\n", style.Bold(s.ProductName), s.FormattedTotal); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Sale ID: %s\n", s.ID); err != nil {
					return err
				}
				if s.OrderID > 0 {
					if err := output.Writef(opts.Out(), "Order: %d\n", s.OrderID); err != nil {
						return err
					}
				}
				if err := output.Writef(opts.Out(), "Buyer: %s\n", s.Email); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Date: %s\n", s.CreatedAt); err != nil {
					return err
				}
				if s.Refunded {
					return output.Writeln(opts.Out(), "Status: "+style.Red("refunded"))
				} else if s.Shipped {
					return output.Writeln(opts.Out(), "Status: "+style.Green("shipped"))
				}

				return nil
			})
		},
	}
}
