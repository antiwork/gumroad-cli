package announcements

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type listResponse struct {
	Campaigns []campaign `json:"campaigns"`
}

type previewEmail struct {
	From     string   `json:"from"`
	ReplyTo  []string `json:"reply_to"`
	Subject  string   `json:"subject"`
	HTMLBody string   `json:"html_body"`
}

type previewResponse struct {
	Campaign        campaign     `json:"campaign"`
	SubscribedCount int          `json:"subscribed_count"`
	Email           previewEmail `json:"email"`
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List feature announcement campaigns",
		Example: `  gumroad admin announcements list`,
		Args:    cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return admincmd.RunGetDecoded[listResponse](opts, "Fetching campaigns...", basePath, url.Values{}, func(resp listResponse) error {
				return renderList(opts, resp.Campaigns)
			})
		},
	}
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "view <key>",
		Short:   "Show a campaign and its delivery counts",
		Example: `  gumroad admin announcements view new_feature`,
		Args:    cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return admincmd.RunGetDecoded[campaignResponse](opts, "Fetching campaign...", cmdutil.JoinPath(basePath, args[0]), url.Values{}, func(resp campaignResponse) error {
				return renderCampaign(opts, "", resp)
			})
		},
	}
}

func newPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview <key>",
		Short: "Render a campaign's email without sending it",
		Long: `Render the campaign's email as your admin account would get it. Nothing is
sent or recorded. Pass the printed content digest to approve.`,
		Example: `  gumroad admin announcements preview new_feature
  gumroad admin announcements preview new_feature --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath(basePath, args[0], "preview")
			return admincmd.RunGetDecoded[previewResponse](opts, "Rendering campaign...", path, url.Values{}, func(resp previewResponse) error {
				return renderPreview(opts, resp)
			})
		},
	}
}

func renderList(opts cmdutil.Options, campaigns []campaign) error {
	if opts.PlainOutput {
		rows := make([][]string, 0, len(campaigns))
		for _, c := range campaigns {
			rows = append(rows, []string{c.Key, c.State, c.Audience, c.Subject})
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if opts.Quiet {
		return nil
	}
	if len(campaigns) == 0 {
		return output.Writeln(opts.Out(), "No campaigns found.")
	}

	tbl := output.NewStyledTable(opts.Style(), "KEY", "STATE", "AUDIENCE", "SUBJECT")
	for _, c := range campaigns {
		tbl.AddRow(output.EscapePlainField(c.Key), output.EscapePlainField(c.State), output.EscapePlainField(c.Audience), output.EscapePlainField(c.Subject))
	}
	return tbl.Render(opts.Out())
}

func renderPreview(opts cmdutil.Options, resp previewResponse) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{resp.Campaign.Key, resp.Campaign.State, resp.Campaign.ContentDigest, resp.Email.Subject}})
	}
	if opts.Quiet {
		return nil
	}

	replyTo := ""
	if len(resp.Email.ReplyTo) > 0 {
		replyTo = resp.Email.ReplyTo[0]
	}
	w := opts.Out()
	if err := writeFields(w, [][2]string{
		{"Key", resp.Campaign.Key},
		{"State", resp.Campaign.State},
		{"Audience", resp.Campaign.Audience},
		{"From", resp.Email.From},
		{"Reply-To", replyTo},
		{"Subject", resp.Email.Subject},
		{"Content digest", resp.Campaign.ContentDigest},
	}); err != nil {
		return err
	}
	if err := output.Writef(w, "Subscribed accounts: %d\n\n", resp.SubscribedCount); err != nil {
		return err
	}
	return writeLines(w, resp.Email.HTMLBody)
}
