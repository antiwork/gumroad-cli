package emails

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type viewEmailResponse struct {
	Email emailRecord `json:"email"`
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View an audience email",
		Long:  "View an audience email, including its state, audience, send setting, URL, and publish time.",
		Example: `  gumroad emails view <id>
  gumroad emails view <id> --json
  gumroad emails view <id> --plain`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[viewEmailResponse](opts, "Fetching email...", "GET", cmdutil.JoinPath("emails", args[0]), url.Values{}, func(resp viewEmailResponse) error {
				return renderEmailView(opts, resp.Email)
			})
		},
	}
}

func renderEmailView(opts cmdutil.Options, item emailRecord) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			item.ID,
			item.Subject,
			item.State,
			item.AudienceType,
			emailBool(item.SendEmails),
			item.URL,
			emailDisplayDate(item),
		}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold(item.Subject)); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "ID: %s\n", item.ID); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "State: %s\n", item.State); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Audience: %s\n", item.AudienceType); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Send emails: %s\n", emailBool(item.SendEmails)); err != nil {
		return err
	}
	if item.URL != "" {
		if err := output.Writef(opts.Out(), "URL: %s\n", item.URL); err != nil {
			return err
		}
	}
	if item.State == emailStateScheduled && item.ScheduledAt != "" {
		return output.Writef(opts.Out(), "Scheduled at: %s\n", item.ScheduledAt)
	}
	if item.PublishedAt != "" {
		return output.Writef(opts.Out(), "Published at: %s\n", item.PublishedAt)
	}
	return nil
}
