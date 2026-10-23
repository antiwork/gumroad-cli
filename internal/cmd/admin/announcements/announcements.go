package announcements

import (
	"io"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const basePath = "feature_announcement_campaigns"

type campaign struct {
	Key           string `json:"key"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	ReplyTo       string `json:"reply_to"`
	Audience      string `json:"audience"`
	State         string `json:"state"`
	ContentDigest string `json:"content_digest"`
	ApprovedBy    string `json:"approved_by"`
	ApprovedAt    string `json:"approved_at"`
	CreatedAt     string `json:"created_at"`
}

type deliveryCount struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type campaignResponse struct {
	Campaign   campaign        `json:"campaign"`
	Deliveries []deliveryCount `json:"deliveries"`
}

func NewAnnouncementsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "announcements",
		Short: "Create and send feature announcement emails",
		Long: `Create and send Gumroad feature announcement emails to opted-in accounts.

A campaign starts as a draft. Preview it, approve the previewed copy with its
content digest, then send it. Approval freezes the copy. Each email still goes
through the server's final checks for consent, account state and audience.`,
		Example: `  gumroad admin announcements create new_feature --audience creators --subject "Try it" --body-file body.html
  gumroad admin announcements preview new_feature
  gumroad admin announcements approve new_feature --content-digest <digest>
  gumroad admin announcements send new_feature
  gumroad admin announcements view new_feature`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newPreviewCmd())
	cmd.AddCommand(newTransitionCmd(approveTransition))
	cmd.AddCommand(newTransitionCmd(sendTransition))
	cmd.AddCommand(newTransitionCmd(cancelTransition))

	return cmd
}

func renderCampaign(opts cmdutil.Options, headline string, resp campaignResponse) error {
	c := resp.Campaign
	if opts.PlainOutput {
		rows := [][]string{{c.Key, c.State, c.Audience, c.Subject, c.ContentDigest}}
		for _, d := range resp.Deliveries {
			rows = append(rows, []string{"delivery", d.State, d.Reason, strconv.Itoa(d.Count)})
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if opts.Quiet {
		return nil
	}

	w := opts.Out()
	style := opts.Style()
	if headline != "" {
		if err := output.Writeln(w, style.Green(headline)); err != nil {
			return err
		}
	}
	if err := writeFields(w, [][2]string{
		{"Key", c.Key},
		{"State", c.State},
		{"Audience", c.Audience},
		{"Subject", c.Subject},
		{"Reply-To", c.ReplyTo},
		{"Content digest", c.ContentDigest},
		{"Approved by", c.ApprovedBy},
		{"Approved at", c.ApprovedAt},
	}); err != nil {
		return err
	}
	for _, d := range resp.Deliveries {
		label := d.State
		if d.Reason != "" {
			label += " (" + d.Reason + ")"
		}
		if err := output.Writef(w, "Deliveries %s: %d\n", output.EscapePlainField(label), d.Count); err != nil {
			return err
		}
	}
	return nil
}

func writeLines(w io.Writer, text string) error {
	for _, line := range strings.Split(text, "\n") {
		if err := output.Writeln(w, output.EscapePlainField(line)); err != nil {
			return err
		}
	}
	return nil
}

func writeFields(w io.Writer, fields [][2]string) error {
	for _, f := range fields {
		if f[1] == "" {
			continue
		}
		if err := output.Writef(w, "%s: %s\n", f[0], output.EscapePlainField(f[1])); err != nil {
			return err
		}
	}
	return nil
}
