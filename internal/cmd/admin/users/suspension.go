package users

import (
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type suspensionResponse struct {
	UserID    string `json:"user_id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	AppealURL string `json:"appeal_url"`
}

type suspensionRequest struct {
	Email  string `json:"email,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

func newSuspensionCmd() *cobra.Command {
	var lookup userLookupFlags

	cmd := &cobra.Command{
		Use:   "suspension",
		Short: "View a user's suspension status",
		Long: `View a user's suspension status.

Identify the user with --email or --user-id. When both are supplied, the
server resolves by --user-id.`,
		Example: `  gumroad admin users suspension --email user@example.com
  gumroad admin users suspension --user-id 2245593582708`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := resolveUserLookupTarget(c, lookup)
			if err != nil {
				return err
			}

			identifier := target.identifier()
			return admincmd.RunPostJSONDecoded[suspensionResponse](opts, "Fetching suspension info...", "/users/suspension", suspensionRequest(target), func(resp suspensionResponse) error {
				return renderSuspension(opts, identifier, resp)
			})
		},
	}

	addUserLookupFlags(cmd, &lookup)

	return cmd
}

func renderSuspension(opts cmdutil.Options, identifier string, resp suspensionResponse) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{identifier, resp.Status, resp.UpdatedAt, resp.AppealURL},
		})
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold(identifier)); err != nil {
		return err
	}
	if resp.Status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", resp.Status); err != nil {
			return err
		}
	}
	if resp.UserID != "" && resp.UserID != identifier {
		if err := output.Writef(opts.Out(), "User ID: %s\n", resp.UserID); err != nil {
			return err
		}
	}
	if resp.UpdatedAt != "" {
		if err := output.Writef(opts.Out(), "Updated: %s\n", resp.UpdatedAt); err != nil {
			return err
		}
	}
	if resp.AppealURL != "" {
		return output.Writef(opts.Out(), "Appeal: %s\n", resp.AppealURL)
	}
	return nil
}
