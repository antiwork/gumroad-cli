package affiliates

import (
	"fmt"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type createOptions struct {
	Email                string
	CommissionPercentage int
}

func newCreateCmd() *cobra.Command {
	var co createOptions

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create an affiliate",
		Args:    cmdutil.ExactArgs(0),
		Example: `  gumroad affiliates create --email partner@example.com --commission 20`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)

			params := url.Values{}
			params.Set("email", co.Email)
			if co.CommissionPercentage > 0 {
				params.Set("commission_percentage", fmt.Sprintf("%d", co.CommissionPercentage))
			}

			return cmdutil.RunRequest(opts, "Creating affiliate...", "POST", "/affiliates", params, func(data []byte) error {
				return cmdutil.PrintSuccess(opts, "Affiliate created successfully.")
			})
		},
	}

	cmd.Flags().StringVar(&co.Email, "email", "", "Email of the affiliate")
	cmd.Flags().IntVar(&co.CommissionPercentage, "commission", 0, "Commission percentage (0-100)")

	_ = cmd.MarkFlagRequired("email")

	return cmd
}
