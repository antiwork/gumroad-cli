package auth

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

type tokenOutput struct {
	Token  string             `json:"token"`
	Source config.TokenSource `json:"source,omitempty"`
}

func newTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Print the active seller auth token",
		Args:  cmdutil.ExactArgs(0),
		Example: `  gumroad auth token
  gumroad auth token --json --jq '.token'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			info, err := config.ResolveToken()
			if err != nil {
				return err
			}

			if opts.UsesJSONOutput() {
				return printAuthJSON(opts, tokenOutput{
					Token:  info.Value,
					Source: info.Source,
				})
			}
			if opts.PlainOutput {
				return output.PrintPlain(opts.Out(), [][]string{{info.Value}})
			}
			return output.Writeln(opts.Out(), info.Value)
		},
	}
}
