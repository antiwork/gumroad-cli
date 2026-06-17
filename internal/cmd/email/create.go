package email

import (
	"net/url"
	"os"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type createEmailResponse struct {
	Email emailRecord `json:"email"`
}

func newCreateCmd() *cobra.Command {
	var subject, body, audience, product string
	var draft, send bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an audience email",
		Long: `Create an audience email from an HTML body file.

Emails are drafts by default. Pass --draft=false or --send only when you intend
to publish and send the email to its audience immediately.`,
		Example: `  gumroad email create --subject "New release" --body ./email.html
  gumroad email create --subject "Product update" --body ./email.html --audience product --product <id>
  gumroad email create --subject "Launch now" --body ./email.html --draft=false --yes
  gumroad email create --subject "Check params" --body ./email.html --dry-run`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			if subject == "" {
				return cmdutil.MissingFlagError(c, "--subject")
			}
			if body == "" {
				return cmdutil.MissingFlagError(c, "--body")
			}
			if !emailValidValue(audience, emailValidAudienceValues()) {
				return cmdutil.UsageErrorf(c, "--audience must be one of: %s", strings.Join(emailValidAudienceValues(), ", "))
			}
			if audience == emailAudienceProduct && product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if send && c.Flags().Changed("draft") && draft {
				return cmdutil.UsageErrorf(c, "--send cannot be used with --draft=true")
			}

			html, err := os.ReadFile(body)
			if err != nil {
				return cmdutil.UsageErrorf(c, "--body: cannot read %s: %v", body, err)
			}

			opts := cmdutil.OptionsFrom(c)
			publish := send || (c.Flags().Changed("draft") && !draft)
			if publish {
				ok, err := cmdutil.ConfirmAction(opts, "Send this email to your audience now?")
				if err != nil {
					return err
				}
				if !ok {
					return cmdutil.PrintCancelledAction(opts, "send email to your audience", "")
				}
			}

			params := url.Values{}
			params.Set("subject", subject)
			params.Set("body", string(html))
			params.Set("audience", audience)
			if audience == emailAudienceProduct {
				params.Set("link_id", product)
			}
			if publish {
				params.Set("publish", "true")
			} else if c.Flags().Changed("draft") {
				params.Set("draft", "true")
			}

			return cmdutil.RunRequestDecoded[createEmailResponse](opts,
				"Creating email...", "POST", cmdutil.JoinPath("emails"), params,
				func(resp createEmailResponse) error {
					item := resp.Email
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{{item.ID, item.Subject, item.State}})
					}
					if opts.Quiet {
						return nil
					}
					style := opts.Style()
					return output.Writef(opts.Out(), "%s %s (%s) [%s]\n",
						style.Bold("Created email:"), item.Subject, style.Dim(item.ID), item.State)
				})
		},
	}

	cmd.Flags().StringVar(&subject, "subject", "", "Email subject (required)")
	cmd.Flags().StringVar(&body, "body", "", "Path to an HTML body file (required)")
	cmd.Flags().StringVar(&audience, "audience", emailAudienceAll, "Audience: all, customers, followers, product")
	cmd.Flags().StringVar(&product, "product", "", "Product ID when --audience product")
	cmd.Flags().BoolVar(&draft, "draft", true, "Create as a draft")
	cmd.Flags().BoolVar(&send, "send", false, "Publish and send immediately")

	return cmd
}
