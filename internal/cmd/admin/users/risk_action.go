package users

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

type riskActionResponse struct {
	Success bool   `json:"success"`
	UserID  string `json:"user_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func renderRiskAction(opts cmdutil.Options, label, identifier string, resp riskActionResponse) error {
	message := resp.Message
	if message == "" {
		message = resp.Status
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{"true", message, identifier, resp.Status},
		})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(message)); err != nil {
		return err
	}
	if err := writeIdentifierLine(opts.Out(), label, message, identifier); err != nil {
		return err
	}
	if resp.Status != "" {
		return output.Writef(opts.Out(), "Status: %s\n", resp.Status)
	}
	return nil
}
