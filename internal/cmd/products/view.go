package products

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func renderProductImage(opts cmdutil.Options, thumbnailURL, previewURL *string) {
	if opts.NoImage || !opts.Style().Enabled() {
		return
	}
	imageURL := ""
	if thumbnailURL != nil && *thumbnailURL != "" {
		imageURL = *thumbnailURL
	} else if previewURL != nil && *previewURL != "" {
		imageURL = *previewURL
	}
	if imageURL == "" {
		return
	}
	maxW := output.TerminalWidthFor(opts.Out(), 40)
	if maxW < 24 {
		return
	}
	if maxW > 64 {
		maxW = 64
	}
	output.RenderImageWithContext(opts.Context, opts.Out(), imageURL, maxW)
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "view <id>",
		Short:   "View a product",
		Args:    cmdutil.ExactArgs(1),
		Example: `  gumroad products view <id>`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequest(opts, "Fetching product...", "GET", cmdutil.JoinPath("products", args[0]), url.Values{}, func(data json.RawMessage) error {
				var resp struct {
					Product struct {
						ID                 string      `json:"id"`
						Name               string      `json:"name"`
						Published          bool        `json:"published"`
						Description        string      `json:"description"`
						FormattedPrice     string      `json:"formatted_price"`
						SalesCount         api.JSONInt `json:"sales_count"`
						SalesUSDCents      float64     `json:"sales_usd_cents"`
						URL                string      `json:"short_url"`
						ThumbnailURL       *string     `json:"thumbnail_url"`
						PreviewURL         *string     `json:"preview_url"`
						IsTieredMembership bool        `json:"is_tiered_membership"`
					} `json:"product"`
				}
				if err := json.Unmarshal(data, &resp); err != nil {
					return fmt.Errorf("could not parse response: %w", err)
				}

				p := resp.Product
				style := opts.Style()

				if opts.PlainOutput {
					status := "draft"
					if p.Published {
						status = "published"
					}
					return output.PrintPlain(opts.Out(), [][]string{
						{p.ID, p.Name, status, p.FormattedPrice, fmt.Sprintf("%d", p.SalesCount), p.URL},
					})
				}

				status := style.Yellow("draft")
				if p.Published {
					status = style.Green("published")
				}

				renderProductImage(opts, p.ThumbnailURL, p.PreviewURL)

				if err := output.Writeln(opts.Out(), style.Bold(p.Name)); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "ID: %s  Status: %s  Price: %s\n", p.ID, status, p.FormattedPrice); err != nil {
					return err
				}
				countLabel := "Sales"
				if p.IsTieredMembership {
					countLabel = "Members"
				}
				if err := output.Writef(opts.Out(), "%s: %d\n", countLabel, p.SalesCount); err != nil {
					return err
				}
				if err := output.Writef(opts.Out(), "Revenue: $%.2f\n", p.SalesUSDCents/100); err != nil {
					return err
				}
				if p.URL != "" {
					if err := output.Writef(opts.Out(), "URL: %s\n", p.URL); err != nil {
						return err
					}
				}
				if p.Description != "" {
					if err := output.Writeln(opts.Out(), style.Dim("\n"+p.Description)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}
