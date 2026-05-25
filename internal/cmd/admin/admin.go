package admin

import (
	adminlicenses "github.com/antiwork/gumroad-cli/internal/cmd/admin/licenses"
	adminpayouts "github.com/antiwork/gumroad-cli/internal/cmd/admin/payouts"
	adminproducts "github.com/antiwork/gumroad-cli/internal/cmd/admin/products"
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
  gumroad admin purchases lookup --stripe-fingerprint <fingerprint>
  gumroad admin purchases refund <purchase-id> --email <email>
  gumroad admin purchases cancel-subscription <purchase-id> --email <email>
  gumroad admin licenses lookup --key <license-key>
  gumroad admin users info --email <email>
  gumroad admin users affiliates --user-id <user-id> --direction granted
  gumroad admin users comments list --user-id <user-id>
  gumroad admin users comments add --user-id <user-id> --content "VAT exempt confirmed"
  gumroad admin users compliance --user-id <user-id>
  gumroad admin users radar --user-id <user-id>
  gumroad admin users purchases --user-id <user-id> --status successful
  gumroad admin users related --email <email> --signal ip
  gumroad admin users suspension --email <email>
  gumroad admin users suspend-for-tos-violation --user-id <user-id> --expected-email <email>
  gumroad admin users watch --user-id <user-id> --expected-email <email> --revenue-threshold 200
  gumroad admin payouts list --email <email>
  gumroad admin payouts pause --user-id <user-id> --expected-email <email>
  gumroad admin products list --email <email>
  gumroad admin products view <product-id>
  gumroad admin products flag-for-tos-violation <product-id> --user-id <user-id>`,
	}

	cmd.AddCommand(adminpurchases.NewPurchasesCmd())
	cmd.AddCommand(adminlicenses.NewLicensesCmd())
	cmd.AddCommand(adminusers.NewUsersCmd())
	cmd.AddCommand(adminpayouts.NewPayoutsCmd())
	cmd.AddCommand(adminproducts.NewProductsCmd())

	return cmd
}
