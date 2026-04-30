package purchases

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

type purchaseActionResponse struct {
	Success bool   `json:"success"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func renderPurchaseAction(opts cmdutil.Options, purchaseID string, resp purchaseActionResponse) error {
	message := resp.Message
	if message == "" {
		message = resp.Status
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{"true", message, purchaseID, resp.Status},
		})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(message)); err != nil {
		return err
	}
	if resp.Status != "" {
		return output.Writef(opts.Out(), "Status: %s\n", resp.Status)
	}
	return nil
}
