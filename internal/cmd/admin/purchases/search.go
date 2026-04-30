package purchases

import (
	"fmt"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type searchRequest struct {
	Email string `json:"email"`
	Limit int    `json:"limit,omitempty"`
}

type searchResponse struct {
	Purchases []purchase  `json:"purchases"`
	Count     api.JSONInt `json:"count"`
	Limit     api.JSONInt `json:"limit"`
	HasMore   bool        `json:"has_more"`
}

func newSearchCmd() *cobra.Command {
	var (
		email string
		limit int
	)

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search admin purchases by buyer email",
		Long: `Search admin purchases by buyer email. Results are capped server-side
(currently 25). The server endpoint also supports query, purchase_status,
creator_email, license_key, and several card filters; this command
exposes only --email and --limit for now.`,
		Example: `  gumroad admin purchases search --email buyer@example.com
  gumroad admin purchases search --email buyer@example.com --limit 10
  gumroad admin purchases search --email buyer@example.com --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			req := searchRequest{Email: email}
			if c.Flags().Changed("limit") {
				if limit <= 0 {
					return cmdutil.UsageErrorf(c, "--limit must be greater than 0")
				}
				req.Limit = limit
			}

			return admincmd.RunPostJSONDecoded[searchResponse](opts, "Searching purchases...", "/purchases/search", req, func(resp searchResponse) error {
				return renderSearch(opts, email, resp)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Buyer email (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum results to return (server caps at 25)")

	return cmd
}

func renderSearch(opts cmdutil.Options, email string, resp searchResponse) error {
	if opts.PlainOutput {
		rows := make([][]string, 0, len(resp.Purchases))
		for _, p := range resp.Purchases {
			rows = append(rows, []string{p.ID, p.Email, productLabel(p), amountLabel(p), statusLabel(p), p.CreatedAt})
		}
		return output.PrintPlain(opts.Out(), rows)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if len(resp.Purchases) == 0 {
		return output.Writef(opts.Out(), "No purchases found for %s.\n", email)
	}

	headline := fmt.Sprintf("%d purchase(s) for %s", len(resp.Purchases), email)
	if resp.HasMore {
		headline = fmt.Sprintf("Showing first %d purchase(s) for %s (truncated)", len(resp.Purchases), email)
	}
	if err := output.Writeln(opts.Out(), style.Bold(headline)); err != nil {
		return err
	}

	for i, p := range resp.Purchases {
		if i > 0 {
			if err := output.Writeln(opts.Out(), ""); err != nil {
				return err
			}
		}
		if err := renderPurchase(opts, p); err != nil {
			return err
		}
	}
	return nil
}
