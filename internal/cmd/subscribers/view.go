package subscribers

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View a subscriber",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequest(opts, "Fetching subscriber...", "GET", cmdutil.JoinPath("subscribers", args[0]), url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					Subscriber struct {
						ID          string `json:"id"`
						Email       string `json:"email_address"`
						Status      string `json:"status"`
						ProductName string `json:"product_name"`
						CreatedAt   string `json:"created_at"`
					} `json:"subscriber"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				s := resp.Subscriber
				style := opts.Style()

				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{
						{s.ID, s.Email, s.Status, s.ProductName, s.CreatedAt},
					})
				}

				status := s.Status
				switch s.Status {
				case "alive":
					status = style.Green(s.Status)
				case "cancelled":
					status = style.Red(s.Status)
				}

				if err := output.Writef(opts.Out(), "%s  %s\n", style.Bold(s.Email), status); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "ID: %s\n", s.ID); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Product: %s\n", s.ProductName); err != nil {
					return err
				}
				return output.Writef(opts.Out(), "Subscribed: %s\n", s.CreatedAt)
			})
		},
	}
}
