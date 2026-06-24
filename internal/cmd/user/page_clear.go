package user

import (
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newPageClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear your profile landing page",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			ok, err := cmdutil.ConfirmAction(opts, "Clear your profile landing page?")
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "clear profile landing page", "")
			}

			target := pageutil.ProfileTarget()
			err = cmdutil.RunRequestDecoded[pageutil.ProfileUpdateResponse](
				opts,
				"Clearing page...",
				http.MethodPut,
				target.Path,
				pageutil.ClearParams(),
				func(resp pageutil.ProfileUpdateResponse) error {
					return pageutil.RenderSanitizationResult(opts, pageutil.RenderResult{
						Action:       "Cleared page",
						BeforeHTML:   pageutil.ProfilePreviousHTML(resp),
						AfterHTML:    resp.CustomHTML,
						LandingURL:   resp.ProfileURL,
						Report:       resp.SanitizationReport,
						ClearMessage: "Page cleared.",
					})
				},
			)
			return pageutil.TranslateRateLimitError(err, pageutil.ProfileClearRateLimitMessage)
		},
	}
}
