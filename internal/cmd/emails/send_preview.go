package emails

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

func newSendPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send-preview <id>",
		Short: "Send a preview email to yourself",
		Long:  "Send a preview of an audience email to your own inbox and print the preview URL, so you can review it before sending to the whole audience.",
		Example: `  gumroad emails send-preview <id>
  gumroad emails send-preview <id> --plain
  gumroad emails send-preview <id> --json`,
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
