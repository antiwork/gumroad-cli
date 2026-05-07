package users

import (
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type suspensionResponse struct {
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	AppealURL string `json:"appeal_url"`
}

type suspensionRequest struct {
	Email      string `json:"email,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

func newSuspensionCmd() *cobra.Command {
	var (
		email      string
		externalID string
	)

	cmd := &cobra.Command{
		Use:   "suspension",
		Short: "View a user's suspension status",
		Long: `View a user's suspension status.

Identify the user with --email or --external-id. When both are supplied, the
server resolves by --external-id.`,
		Example: `  gumroad admin users suspension --email user@example.com
  gumroad admin users suspension --external-id 2245593582708`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := requireEmailOrExternalID(c, email, externalID); err != nil {
				return err
			}

			identifier := userIdentifier(email, externalID)
			return admincmd.RunPostJSONDecoded[suspensionResponse](opts, "Fetching suspension info...", "/users/suspension", suspensionRequest{Email: email, ExternalID: externalID}, func(resp suspensionResponse) error {
				return renderSuspension(opts, identifier, resp)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&externalID, "external-id", "", "User external ID")

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
