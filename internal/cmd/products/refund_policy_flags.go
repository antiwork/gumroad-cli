package products

import (
	"net/url"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// Allowed values for --refund-period on products create/update. "inherit"
// disables the product-level override so the account default applies again;
// the numeric values match the account-level refund-policy periods.
var allowedProductRefundPeriods = []string{"inherit", "none", "7", "14", "30", "183"}

func registerProductRefundPolicyFlags(cmd *cobra.Command, refundPeriod, refundFinePrint *string) {
	cmd.Flags().StringVar(refundPeriod, "refund-period", "", "Product-level refund period: inherit, none, 7, 14, 30, or 183 (inherit returns to the account default)")
	cmd.Flags().StringVar(refundFinePrint, "refund-fine-print", "", "Fine print for the product-level refund policy (empty string clears it)")
	_ = cmd.RegisterFlagCompletionFunc("refund-period", productRefundPeriodCompletion)
}

func productRefundPeriodCompletion(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return allowedProductRefundPeriods, cobra.ShellCompDirectiveNoFileComp
}

func validateProductRefundPolicyFlags(c *cobra.Command, refundPeriod string) error {
	flags := c.Flags()
	if flags.Changed("refund-period") {
		valid := false
		for _, allowed := range allowedProductRefundPeriods {
			if refundPeriod == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return cmdutil.UsageErrorf(c, "--refund-period must be one of: %s", strings.Join(allowedProductRefundPeriods, ", "))
		}
		if refundPeriod == "inherit" && flags.Changed("refund-fine-print") {
			return cmdutil.UsageErrorf(c, "--refund-fine-print cannot be combined with --refund-period inherit; the account-level policy's fine print applies")
		}
	}
	return nil
}

func setProductRefundPolicyParams(c *cobra.Command, params url.Values, refundPeriod, refundFinePrint string) {
	flags := c.Flags()
	if flags.Changed("refund-period") {
		params.Set("refund_period", refundPeriod)
	}
	if flags.Changed("refund-fine-print") {
		params.Set("refund_fine_print", refundFinePrint)
	}
}
