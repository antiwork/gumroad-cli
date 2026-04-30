package discover

import "github.com/spf13/cobra"

func NewDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Browse public Gumroad products",
		Long: `Browse and search public products on Gumroad without authentication.

Discover wraps the same public search index that powers gumroad.com/discover, so
results match what shoppers see on the web.`,
		Example: `  gumroad discover search "machine learning"
  gumroad discover search --tag productivity --max-price 30
  gumroad discover search "design" --json --jq '.products[].name'`,
	}

	cmd.AddCommand(newSearchCmd())
	return cmd
}
