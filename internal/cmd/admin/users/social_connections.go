package users

import (
	"fmt"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type socialConnectionsResponse struct {
	SocialConnections []socialConnection `json:"social_connections"`
}

type socialConnection struct {
	Platform                string `json:"platform"`
	UID                     string `json:"uid"`
	Handle                  string `json:"handle"`
	CurrentlyLinked         bool   `json:"currently_linked"`
	AccountCreatedAt        string `json:"account_created_at"`
	FollowerCount           *int64 `json:"follower_count"`
	PostCount               *int64 `json:"post_count"`
	LastPostedAt            string `json:"last_posted_at"`
	LastVerifiedAt          string `json:"last_verified_at"`
	SharedIdentityUserCount int    `json:"shared_identity_user_count"`
}

func newSocialConnectionsCmd() *cobra.Command {
	var lookup userLookupFlags
	cmd := &cobra.Command{
		Use:   "social-connections",
		Short: "Read verified social evidence for risk review",
		Long: `Read stored social verification evidence, including current-link state,
verification timestamps, audience history, and shared identities. Historical
verification is not proof of a current connection. Missing counts are unknown,
not zero. This read-only command does not refresh providers, score eligibility,
mark a seller compliant, or release payouts.`,
		Example: `  gumroad admin users social-connections --user-id 2245593582708
  gumroad admin users social-connections --email seller@example.com --json`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}
			return admincmd.RunGetDecoded[socialConnectionsResponse](opts, "Fetching social evidence...", "/users/social_connections", target.Values(), func(resp socialConnectionsResponse) error {
				return renderSocialConnections(opts, resp)
			})
		},
	}
	addUserLookupFlags(cmd, &lookup)
	return cmd
}

func renderSocialConnections(opts cmdutil.Options, resp socialConnectionsResponse) error {
	rows := make([][]string, 0, len(resp.SocialConnections))
	for _, v := range resp.SocialConnections {
		rows = append(rows, []string{v.Platform, v.UID, v.Handle, strconv.FormatBool(v.CurrentlyLinked),
			v.LastVerifiedAt, v.AccountCreatedAt, socialCount(v.FollowerCount), socialCount(v.PostCount),
			v.LastPostedAt, strconv.Itoa(v.SharedIdentityUserCount)})
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), rows)
	}
	if opts.Quiet {
		return nil
	}
	if len(rows) == 0 {
		return cmdutil.PrintInfo(opts, "No stored social verification evidence.")
	}
	if err := output.Writeln(opts.Out(), "Stored social evidence (not payout approval):"); err != nil {
		return err
	}
	labels := []string{"Platform", "Identity", "Handle", "Currently linked", "Last verified", "Account created", "Followers", "Posts", "Last posted", "Other users sharing identity"}
	for _, row := range rows {
		for i, value := range row {
			if err := output.Writef(opts.Out(), "%s: %s\n", labels[i], output.EscapePlainField(fallback(value, "unknown"))); err != nil {
				return err
			}
		}
		if err := output.Writeln(opts.Out(), ""); err != nil {
			return err
		}
	}
	return nil
}

func socialCount(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprint(*value)
}
