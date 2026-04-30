package payouts

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

type payoutsActionResponse struct {
	Success       bool   `json:"success"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	PayoutsPaused bool   `json:"payouts_paused"`
}

func renderPayoutsAction(opts cmdutil.Options, email string, resp payoutsActionResponse) error {
	message := resp.Message
	if message == "" {
		message = resp.Status
	}

	state := "resumed"
	if resp.PayoutsPaused {
		state = "paused"
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{"true", message, email, resp.Status, state},
		})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Green(message)); err != nil {
		return err
	}
	if email != "" {
		if err := output.Writef(opts.Out(), "Email: %s\n", email); err != nil {
			return err
		}
	}
	if resp.Status != "" {
		if err := output.Writef(opts.Out(), "Status: %s\n", resp.Status); err != nil {
			return err
		}
	}
	return output.Writef(opts.Out(), "Payouts: %s\n", state)
}
