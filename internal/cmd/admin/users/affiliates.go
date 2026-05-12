package users

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const maxAffiliateLimit = 100

var validAffiliateDirections = map[string]bool{
	"granted":  true,
	"received": true,
}

type affiliatesResponse struct {
	UserID     string              `json:"user_id"`
	Direction  string              `json:"direction"`
	Affiliates []affiliateRelation `json:"affiliates"`
	Pagination cursor.Pagination   `json:"pagination"`
}

type affiliateRelation struct {
	ID                   string             `json:"id"`
	Type                 string             `json:"type"`
	Direction            string             `json:"direction"`
	Counterparty         affiliateUser      `json:"counterparty"`
	AffiliateBasisPoints api.JSONInt        `json:"affiliate_basis_points"`
	DestinationURL       string             `json:"destination_url"`
	ApplyToAllProducts   bool               `json:"apply_to_all_products"`
	Alive                bool               `json:"alive"`
	DeletedAt            string             `json:"deleted_at"`
	CreatedAt            string             `json:"created_at"`
	Products             []affiliateProduct `json:"products"`
}

type affiliateUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type affiliateProduct struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	BasisPoints    api.JSONInt `json:"basis_points"`
	DestinationURL string      `json:"destination_url"`
}

func newAffiliatesCmd() *cobra.Command {
	var (
		lookup    userLookupFlags
		page      cursor.Flags
		direction string
	)

	cmd := &cobra.Command{
		Use:   "affiliates",
		Short: "List affiliate relationships for a user",
		Long: `List affiliate relationships for a user in one direction. Use
--direction granted when the user is the seller granting affiliate access, or
--direction received when the user is the affiliate receiving access.`,
		Example: `  gumroad admin users affiliates --user-id 2245593582708 --direction granted
  gumroad admin users affiliates --email user@example.com --direction received --limit 50
  gumroad admin users affiliates --email user@example.com --direction received --cursor cur-next`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}
			if direction == "" {
				return cmdutil.MissingFlagError(c, "--direction")
			}
			if !validAffiliateDirections[direction] {
				return cmdutil.UsageErrorf(c, "--direction must be one of: granted, received")
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", page.Limit); err != nil {
				return err
			}
			if c.Flags().Changed("limit") && page.Limit > maxAffiliateLimit {
				return cmdutil.UsageErrorf(c, "--limit must be %d or less", maxAffiliateLimit)
			}

			params := target.values()
			params.Set("direction", direction)
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[affiliatesResponse](opts, "Fetching user affiliates...", "/users/affiliates", params, func(resp affiliatesResponse) error {
				return renderAffiliates(opts, target.identifier(), direction, resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)
	cmd.Flags().StringVar(&direction, "direction", "", "Affiliate direction: granted or received (required)")
	cursor.AddFlags(cmd, &page, cursor.Options{LimitUsage: "Maximum results to return (default 20, capped at 100)"})

	return cmd
}

func renderAffiliates(opts cmdutil.Options, identifier, direction string, resp affiliatesResponse) error {
	if opts.PlainOutput {
		return writeAffiliatesPlain(opts.Out(), resp.Affiliates)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.Affiliates) == 0 {
			if err := output.Writef(w, "No %s affiliate relationships found for %s.\n", direction, identifier); err != nil {
				return err
			}
			return cursor.WriteMoreFooter(w, resp.Pagination)
		}

		headline := fmt.Sprintf("%d %s affiliate relationship(s) for %s", len(resp.Affiliates), direction, identifier)
		if err := output.Writeln(w, style.Bold(headline)); err != nil {
			return err
		}
		if resp.UserID != "" && resp.UserID != identifier {
			if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
				return err
			}
		}
		if err := output.Writeln(w, ""); err != nil {
			return err
		}
		if err := writeAffiliatesTable(w, style, resp.Affiliates); err != nil {
			return err
		}
		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writeAffiliatesPlain(w io.Writer, affiliates []affiliateRelation) error {
	rows := make([][]string, 0, len(affiliates))
	for _, a := range affiliates {
		rows = append(rows, []string{
			a.ID,
			a.Type,
			affiliateCounterpartyLabel(a.Counterparty),
			strconv.Itoa(int(a.AffiliateBasisPoints)),
			affiliateProductsLabel(a),
			strconv.FormatBool(a.Alive),
			a.CreatedAt,
		})
	}
	return output.PrintPlain(w, rows)
}

func writeAffiliatesTable(w io.Writer, style output.Styler, affiliates []affiliateRelation) error {
	tbl := output.NewStyledTable(style, "ID", "TYPE", "COUNTERPARTY", "BPS", "PRODUCTS", "ALIVE", "CREATED")
	for _, a := range affiliates {
		tbl.AddRow(
			a.ID,
			a.Type,
			affiliateCounterpartyLabel(a.Counterparty),
			strconv.Itoa(int(a.AffiliateBasisPoints)),
			affiliateProductsLabel(a),
			strconv.FormatBool(a.Alive),
			a.CreatedAt,
		)
	}
	return tbl.Render(w)
}

func affiliateCounterpartyLabel(user affiliateUser) string {
	parts := make([]string, 0, 3)
	if user.Email != "" {
		parts = append(parts, user.Email)
	}
	if user.Name != "" && user.Name != user.Email {
		parts = append(parts, user.Name)
	}
	if user.ID != "" {
		parts = append(parts, user.ID)
	}
	return strings.Join(parts, " / ")
}

func affiliateProductsLabel(a affiliateRelation) string {
	if a.ApplyToAllProducts {
		return "all products"
	}
	if len(a.Products) == 0 {
		return ""
	}

	labels := make([]string, 0, len(a.Products))
	for _, p := range a.Products {
		labels = append(labels, affiliateProductLabel(p))
	}
	return strings.Join(labels, ", ")
}

func affiliateProductLabel(p affiliateProduct) string {
	switch {
	case p.Name != "" && p.ID != "":
		return fmt.Sprintf("%s (%s)", p.Name, p.ID)
	case p.Name != "":
		return p.Name
	default:
		return p.ID
	}
}
