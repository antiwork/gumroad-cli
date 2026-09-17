package marketing

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type actionRecord struct {
	ID                string `json:"id"`
	IdempotencyKey    string `json:"idempotency_key"`
	ConfirmationToken string `json:"confirmation_token"`
	Status            string `json:"status"`
	PostText          string `json:"post_text"`
	LinkURL           string `json:"link_url"`
	ExternalURL       string `json:"external_url"`
	ErrorCode         string `json:"error_code"`
}

type actionResponse struct {
	Action actionRecord `json:"marketing_action"`
	Handle string       `json:"handle"`
}

type recommendationsResponse struct {
	Channels []struct {
		Channel string       `json:"channel"`
		Live    bool         `json:"live"`
		Handle  string       `json:"handle"`
		Action  actionRecord `json:"action"`
	} `json:"channels"`
}

func NewMarketingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "marketing",
		Short:   "Review and confirm launch posts",
		Long:    "Use the seller's existing marketing actions. Requires auto_marketing and an edit_emails OAuth token. No local marketing state or scheduler is created.",
		Example: "  gumroad marketing recommend <product> --json\n  gumroad marketing approve <action>\n  gumroad marketing schedule <action>\n  gumroad marketing status <action>\n  gumroad marketing cancel <action> --yes",
	}
	cmd.AddCommand(newRecommendCmd(), newStatusCmd(), newActionCmd("approve", "approve"), newActionCmd("schedule", "execute"), newActionCmd("cancel", "cancel"))
	return cmd
}

func newRecommendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recommend <product>",
		Short: "Prepare the server's launch recommendations",
		Long:  "Return the existing channel picker and reuse its open actions and tagged links. Accepts a product ID or permalink. This GET may create the recommendation on the server; nothing is posted.",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[recommendationsResponse](opts, "Preparing recommendations...", http.MethodGet, cmdutil.JoinPath("products", args[0], "marketing", "recommendations"), nil, func(resp recommendationsResponse) error {
				if opts.Quiet && !opts.PlainOutput {
					return nil
				}
				rows := [][]string{}
				for _, channel := range resp.Channels {
					state := "coming soon"
					if channel.Live {
						state = channel.Action.Status
					}
					rows = append(rows, []string{channel.Channel, channel.Action.ID, state, channel.Handle, channel.Action.PostText, channel.Action.LinkURL})
				}
				return output.PrintPlain(opts.Out(), rows)
			})
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <action>",
		Short: "Show a marketing action and its posting result",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[actionResponse](opts, "Fetching action...", http.MethodGet, cmdutil.JoinPath("marketing_actions", args[0]), nil, func(resp actionResponse) error { return renderAction(opts, resp) })
		},
	}
}

func newActionCmd(verb, operation string) *cobra.Command {
	var expectedToken string
	descriptions := map[string]string{
		"approve":  "Approve this action without posting. Review the exact text, account handle, and link first.",
		"schedule": "Execute an approved action now through the server's channel executor. This version does not support future dates: schedule posts immediately, not later. Approve first. Repeating it resolves the same action, never a second post.",
		"cancel":   "Cancel an unclaimed action. A post already claimed or published cannot be recalled.",
	}
	cmd := &cobra.Command{
		Use:   verb + " <action>",
		Short: descriptions[verb],
		Long:  descriptions[verb] + "\n\nThe confirmation preview uses quoted strings so control characters cannot hide content. --yes explicitly confirms for scripts and MCP clients; approve and schedule also require --confirmation-token from the preview the seller reviewed. --dry-run still fetches the action for its preview, but sends no mutation.",
		Args:  cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := cmdutil.JoinPath("marketing_actions", args[0])
			resp, err := cmdutil.FetchRequestDecoded[actionResponse](opts, "Fetching action...", http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			item := resp.Action
			if item.ID != args[0] || item.IdempotencyKey == "" || (verb != "cancel" && item.ConfirmationToken == "") {
				return cmdutil.InvalidInputErrorf("The server did not return this action and its idempotency key.")
			}
			if verb != "cancel" && (resp.Handle == "" || item.PostText == "" || item.LinkURL == "") {
				return cmdutil.InvalidInputErrorf("The action is missing its account, text, or link. Connect X and request a recommendation first.")
			}
			if err := output.Writef(opts.Err(), "Account: @%s\nExact post text: %q\nLink: %q\n", output.EscapePlainField(resp.Handle), item.PostText, item.LinkURL); err != nil {
				return err
			}
			if opts.Yes && !opts.DryRun && verb != "cancel" && expectedToken == "" {
				return cmdutil.InvalidInputErrorf("With --yes, supply --confirmation-token from the recommendation or status the seller reviewed.")
			}
			if expectedToken != "" && expectedToken != item.ConfirmationToken {
				return cmdutil.InvalidInputErrorf("The post changed. Review its text, account and link and confirm again.")
			}
			message := fmt.Sprintf("%s this action?", verb)
			if verb == "schedule" {
				message = "Post this approved action now?"
			}
			ok, err := cmdutil.ConfirmAction(opts, message)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, verb+" marketing action", args[0])
			}
			params := url.Values{"idempotency_key": {item.IdempotencyKey}, "confirmation_token": {item.ConfirmationToken}}
			return cmdutil.RunRequestDecoded[actionResponse](opts, "Updating action...", http.MethodPost, path+"/"+operation, params, func(result actionResponse) error { return renderAction(opts, result) })
		},
	}
	cmd.Flags().StringVar(&expectedToken, "confirmation-token", "", "Token from the reviewed action (required with --yes for approve and schedule)")
	return cmd
}

func renderAction(opts cmdutil.Options, resp actionResponse) error {
	if opts.Quiet && !opts.PlainOutput {
		return nil
	}
	item := resp.Action
	return output.PrintPlain(opts.Out(), [][]string{{item.ID, item.Status, resp.Handle, item.PostText, item.LinkURL, item.ExternalURL, item.ErrorCode}})
}
