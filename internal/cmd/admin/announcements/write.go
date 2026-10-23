package announcements

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type contentFlags struct {
	subject  string
	body     string
	bodyFile string
	replyTo  string
}

func addContentFlags(cmd *cobra.Command, f *contentFlags) {
	cmd.Flags().StringVar(&f.subject, "subject", "", "Email subject")
	cmd.Flags().StringVar(&f.body, "body", "", "Email body as HTML")
	cmd.Flags().StringVar(&f.bodyFile, "body-file", "", "Read the HTML body from a file")
	cmd.Flags().StringVar(&f.replyTo, "reply-to", "", "Reply-To address at gumroad.com")
}

func (f contentFlags) params(c *cobra.Command) (map[string]string, error) {
	if f.body != "" && f.bodyFile != "" {
		return nil, cmdutil.UsageErrorf(c, "use --body or --body-file, not both")
	}
	params := map[string]string{}
	if c.Flags().Changed("subject") {
		params["subject"] = f.subject
	}
	if c.Flags().Changed("body") {
		params["body"] = f.body
	}
	if f.bodyFile != "" {
		data, err := os.ReadFile(f.bodyFile)
		if err != nil {
			return nil, fmt.Errorf("read --body-file: %w", err)
		}
		params["body"] = string(data)
	}
	if c.Flags().Changed("reply-to") {
		params["reply_to"] = f.replyTo
	}
	return params, nil
}

func newCreateCmd() *cobra.Command {
	var (
		content  contentFlags
		audience string
	)

	cmd := &cobra.Command{
		Use:   "create <key>",
		Short: "Create a draft campaign",
		Long: `Create a draft feature announcement campaign. The key is a lowercase name
such as new_feature. The audience must be one the server knows, for example
creators.`,
		Example: `  gumroad admin announcements create new_feature --audience creators --subject "Try it" --body-file body.html
  gumroad admin announcements create new_feature --audience creators --subject "Try it" --body "<p>Hi</p>" --reply-to support@gumroad.com`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if audience == "" {
				return cmdutil.MissingFlagError(c, "--audience")
			}
			if content.subject == "" {
				return cmdutil.MissingFlagError(c, "--subject")
			}
			params, err := content.params(c)
			if err != nil {
				return err
			}
			if params["body"] == "" {
				return cmdutil.MissingFlagError(c, "--body or --body-file")
			}
			if content.replyTo == "" {
				params["reply_to"] = "support@gumroad.com"
			}
			params["key"] = args[0]
			params["audience"] = audience

			return writeCampaign(opts, http.MethodPost, basePath, params, "Created draft "+args[0])
		},
	}

	addContentFlags(cmd, &content)
	cmd.Flags().StringVar(&audience, "audience", "", "Audience key, such as creators (required)")

	return cmd
}

func newUpdateCmd() *cobra.Command {
	var content contentFlags

	cmd := &cobra.Command{
		Use:     "update <key>",
		Short:   "Change a draft campaign's copy",
		Long:    `Change the subject, body or Reply-To of a draft. The server refuses changes after approval.`,
		Example: `  gumroad admin announcements update new_feature --body-file body.html`,
		Args:    cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			params, err := content.params(c)
			if err != nil {
				return err
			}
			if len(params) == 0 {
				return cmdutil.UsageErrorf(c, "pass at least one of --subject, --body, --body-file or --reply-to")
			}

			return writeCampaign(opts, http.MethodPut, cmdutil.JoinPath(basePath, args[0]), params, "Updated draft "+args[0])
		},
	}

	addContentFlags(cmd, &content)

	return cmd
}

func writeCampaign(opts cmdutil.Options, method, path string, params map[string]string, headline string) error {
	if opts.DryRun {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		return cmdutil.PrintDryRunRequest(opts, method, adminapi.AdminPath(path), values)
	}

	fetch := admincmd.FetchPostJSON
	if method == http.MethodPut {
		fetch = admincmd.FetchPutJSON
	}
	data, err := fetch(opts, "Saving campaign...", path, params)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	decoded, err := cmdutil.DecodeJSON[campaignResponse](data)
	if err != nil {
		return err
	}
	return renderCampaign(opts, headline, decoded)
}
