package admin

import (
	adminlicenses "github.com/antiwork/gumroad-cli/internal/cmd/admin/licenses"
	adminpayouts "github.com/antiwork/gumroad-cli/internal/cmd/admin/payouts"
	adminpurchases "github.com/antiwork/gumroad-cli/internal/cmd/admin/purchases"
	adminusers "github.com/antiwork/gumroad-cli/internal/cmd/admin/users"
	"github.com/spf13/cobra"
)

func NewAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Run Gumroad admin commands",
		Long:  "Run internal Gumroad admin API commands with a separate admin token.",
		Example: `  gumroad admin purchases view <purchase-id>
  gumroad admin purchases refund <purchase-id> --email <email>
  gumroad admin purchases cancel-subscription <purchase-id> --email <email>
  gumroad admin licenses lookup --key <license-key>
  gumroad admin users info --email <email>
  gumroad admin users suspension --email <email>
  gumroad admin payouts list --email <email>
  gumroad admin payouts pause --email <email>`,
	}

	cmd.AddCommand(adminpurchases.NewPurchasesCmd())
	cmd.AddCommand(adminlicenses.NewLicensesCmd())
	cmd.AddCommand(adminusers.NewUsersCmd())
	cmd.AddCommand(adminpayouts.NewPayoutsCmd())

	return cmd
}
