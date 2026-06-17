package email

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type previewEmailResponse struct {
	Success    bool   `json:"success"`
	PreviewURL string `json:"preview_url"`
	Message    string `json:"message"`
}

func newPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview <id>",
		Short: "Send an email preview",
		Long:  "Send an email preview and print the preview URL for review before sending.",
		Example: `  gumroad email preview <id>
  gumroad email preview <id> --plain
  gumroad email preview <id> --json`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[previewEmailResponse](opts, "Sending preview...", "POST", cmdutil.JoinPath("emails", args[0], "preview"), url.Values{}, func(resp previewEmailResponse) error {
				if opts.PlainOutput {
					return output.PrintPlain(opts.Out(), [][]string{{resp.PreviewURL}})
				}
				if opts.Quiet {
					return nil
				}
				message := resp.Message
				if message == "" {
					message = "Preview sent to your email."
				}
				if err := output.Writeln(opts.Out(), message); err != nil {
					return err
				}
				if resp.PreviewURL != "" {
					return output.Writeln(opts.Out(), resp.PreviewURL)
				}
				return nil
			})
		},
	}
}
