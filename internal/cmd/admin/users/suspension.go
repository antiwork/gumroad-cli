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
	Email string `json:"email"`
}

func newSuspensionCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "suspension",
		Short: "View a user's suspension status",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if email == "" {
				return cmdutil.MissingFlagError(c, "--email")
			}

			return admincmd.RunPostJSONDecoded[suspensionResponse](opts, "Fetching suspension info...", "/users/suspension", suspensionRequest{Email: email}, func(resp suspensionResponse) error {
				return renderSuspension(opts, email, resp)
			})
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")

	return cmd
}

func renderSuspension(opts cmdutil.Options, email string, resp suspensionResponse) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{email, resp.Status, resp.UpdatedAt, resp.AppealURL},
		})
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold(email)); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Status: %s\n", resp.Status); err != nil {
		return err
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
