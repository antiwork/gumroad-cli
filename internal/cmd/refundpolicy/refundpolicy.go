package refundpolicy

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const refundPolicyPath = "/refund_policy"

var allowedRefundPeriods = []string{"none", "7", "14", "30", "183"}

type refundPolicy struct {
	RefundPeriod string  `json:"refund_period"`
	Title        string  `json:"title"`
	FinePrint    *string `json:"fine_print"`
	InEffect     bool    `json:"in_effect"`
}

type refundPolicyResponse struct {
	RefundPolicy refundPolicy `json:"refund_policy"`
}

func NewRefundPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refund-policy",
		Short: "View and update the account refund policy",
		Long: "View and update the account-level refund policy shown to buyers.\n\n" +
			"Refund policy is store-wide. Allowed refund periods are none, 7, 14, 30, and 183 days.",
		Example: `  gumroad refund-policy view
  gumroad refund-policy view --json --jq '.refund_policy.refund_period'
  gumroad refund-policy set --period 30 --fine-print "Refund requests are reviewed within 2 business days."
  gumroad refund-policy set --period none --fine-print ""`,
	}

	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newSetCmd())

	return cmd
}

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Show the account refund policy",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			return cmdutil.RunRequestDecoded[refundPolicyResponse](opts,
				"Fetching refund policy...", http.MethodGet, refundPolicyPath, url.Values{},
				func(resp refundPolicyResponse) error {
					return renderRefundPolicy(opts, resp, "")
				})
		},
	}
}

func newSetCmd() *cobra.Command {
	var period, finePrint string

	cmd := &cobra.Command{
		Use:   "set --period <none|7|14|30|183>",
		Short: "Update the account refund policy",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if period == "" {
				return cmdutil.MissingFlagError(c, "--period")
			}
			if err := validateRefundPeriod(c, period); err != nil {
				return err
			}

			params := url.Values{}
			params.Set("refund_period", period)
			if c.Flags().Changed("fine-print") {
				params.Set("fine_print", finePrint)
			}

			return cmdutil.RunRequestDecoded[refundPolicyResponse](opts,
				"Updating refund policy...", http.MethodPut, refundPolicyPath, params,
				func(resp refundPolicyResponse) error {
					return renderRefundPolicy(opts, resp, "Updated refund policy.")
				})
		},
	}

	cmd.Flags().StringVar(&period, "period", "", "Refund period: none, 7, 14, 30, or 183 (required)")
	cmd.Flags().StringVar(&finePrint, "fine-print", "", "Optional fine print; pass an empty value to clear it")
	_ = cmd.RegisterFlagCompletionFunc("period", refundPeriodCompletion)

	return cmd
}

func validateRefundPeriod(cmd *cobra.Command, period string) error {
	for _, allowed := range allowedRefundPeriods {
		if period == allowed {
			return nil
		}
	}
	return cmdutil.UsageErrorf(cmd, "--period must be one of: %s", strings.Join(allowedRefundPeriods, ", "))
}

func refundPeriodCompletion(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return allowedRefundPeriods, cobra.ShellCompDirectiveNoFileComp
}

func renderRefundPolicy(opts cmdutil.Options, resp refundPolicyResponse, header string) error {
	policy := resp.RefundPolicy

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			policy.RefundPeriod,
			policy.Title,
			finePrintValue(policy.FinePrint),
			strconv.FormatBool(policy.InEffect),
		}})
	}

	if header != "" {
		if opts.Quiet {
			return nil
		}
		if err := output.Writeln(opts.Out(), opts.Style().Green(header)); err != nil {
			return err
		}
	}

	if err := output.Writeln(opts.Out(), opts.Style().Bold("Refund policy")); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Period: %s\n", policy.RefundPeriod); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Title: %s\n", policy.Title); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Fine print: %s\n", displayFinePrint(policy.FinePrint)); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "In effect: %s\n", yesNo(policy.InEffect))
}

func finePrintValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func displayFinePrint(value *string) string {
	if value == nil || *value == "" {
		return "(none)"
	}
	return *value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
