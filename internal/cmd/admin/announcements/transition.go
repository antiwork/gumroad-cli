package announcements

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type transition struct {
	use, short, long, example string
	endpoint                  string
	confirm, spinner, done    string
	needsDigest               bool
}

var approveTransition = transition{
	use:   "approve <key>",
	short: "Approve a draft's copy",
	long: `Approve a draft and freeze its copy. Pass the content digest from preview,
so the server refuses the approval if the copy changed after you saw it.`,
	example:     `  gumroad admin announcements approve new_feature --content-digest <digest>`,
	endpoint:    "approve",
	confirm:     "Approve campaign %s? Its copy can't change after this.",
	spinner:     "Approving campaign...",
	done:        "Approved %s",
	needsDigest: true,
}

var sendTransition = transition{
	use:   "send <key>",
	short: "Send an approved campaign",
	long: `Start sending an approved campaign to every opted-in account. The server
checks consent, account state and audience again before each email.`,
	example:  `  gumroad admin announcements send new_feature`,
	endpoint: "deliver",
	confirm:  "Send campaign %s to every opted-in account in its audience?",
	spinner:  "Starting send...",
	done:     "Sending %s",
}

var cancelTransition = transition{
	use:      "cancel <key>",
	short:    "Cancel a campaign",
	long:     `Cancel a campaign. Emails still in the queue are skipped.`,
	example:  `  gumroad admin announcements cancel new_feature`,
	endpoint: "cancel",
	confirm:  "Cancel campaign %s?",
	spinner:  "Cancelling campaign...",
	done:     "Cancelled %s",
}

func newTransitionCmd(t transition) *cobra.Command {
	var digest string

	cmd := &cobra.Command{
		Use:     t.use,
		Short:   t.short,
		Long:    t.long,
		Example: t.example,
		Args:    cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			key := args[0]
			if t.needsDigest && digest == "" {
				return cmdutil.MissingFlagError(c, "--content-digest")
			}

			ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(t.confirm, key))
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, t.endpoint+" campaign "+key, key)
			}

			path := cmdutil.JoinPath(basePath, key, t.endpoint)
			payload := map[string]string{}
			if t.needsDigest {
				payload["content_digest"] = digest
			}

			if opts.DryRun {
				values := url.Values{}
				for k, v := range payload {
					values.Set(k, v)
				}
				return cmdutil.PrintDryRunRequest(opts, http.MethodPost, adminapi.AdminPath(path), values)
			}

			data, err := admincmd.FetchPostJSON(opts, t.spinner, path, payload)
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
			return renderCampaign(opts, fmt.Sprintf(t.done, key), decoded)
		},
	}

	if t.needsDigest {
		cmd.Flags().StringVar(&digest, "content-digest", "", "Content digest printed by preview (required)")
	}

	return cmd
}
