package pages

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

type pagePullResponse struct {
	Success      bool   `json:"success"`
	Page         page   `json:"page"`
	RenderedHTML string `json:"rendered_html"`
}

type profilePullResponse struct {
	Success        bool   `json:"success"`
	CustomHTML     string `json:"custom_html"`
	RenderedHTML   string `json:"rendered_html"`
	HasLandingPage bool   `json:"has_landing_page"`
	ProfileURL     string `json:"profile_url"`
}

func newPullCmd() *cobra.Command {
	var outputPath string
	var force bool

	cmd := &cobra.Command{
		Use:   "pull <slug>",
		Short: "Pull a storefront page's rendered HTML",
		Long: "Pull the rendered HTML of a storefront page into a local file — the starting point for going custom. " +
			"The file is a faithful standalone render of the page as it serves today, so you start from your current page instead of a blank one.\n\n" +
			"Edit the file, check the result with `gumroad pages preview <file>`, then publish it with `gumroad pages push <slug> <file>`.\n\n" +
			"Use the slug \"profile\" to pull your profile landing page (your store's home page): your published custom HTML if you have one, otherwise the default storefront render.",
		Args: pullArgs,
		Example: `  gumroad pages pull about
  gumroad pages pull about -o custom.html
  gumroad pages pull about -o -
  gumroad pages pull profile
  gumroad pages pull about --json --jq '.page.title'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			slug := args[0]
			dest := outputPath
			if dest == "" {
				// The implicit destination must stay a plain filename in the
				// current directory. Real slugs never contain path separators,
				// so a slug like "../backup/about" would silently write
				// outside the cwd — writing anywhere else requires an
				// explicit -o path.
				if filepath.Base(slug) != slug {
					return cmdutil.InvalidInputErrorf("slug %q looks like a path — use -o to choose where the file goes", slug)
				}
				dest = slug + ".html"
			}
			// Refuse to clobber an existing file before the request so the
			// API call is never wasted. JSON mode prints the raw response and
			// never writes the file, so the check does not apply there.
			if dest != "-" && !force && !opts.UsesJSONOutput() {
				if _, err := os.Stat(dest); err == nil {
					return cmdutil.InvalidInputErrorf("%s already exists (use --force to overwrite, or -o to pick another path)", dest)
				}
			}

			if slug == profileSlug {
				err := cmdutil.RunRequestDecoded[profilePullResponse](
					opts,
					"Pulling page...",
					http.MethodGet,
					pageutil.ProfileTarget().Path,
					url.Values{},
					func(resp profilePullResponse) error {
						return writePulledHTML(opts, slug, dest, resp.RenderedHTML)
					},
				)
				return translatePullError(err, slug)
			}

			err := cmdutil.RunRequestDecoded[pagePullResponse](
				opts,
				"Pulling page...",
				http.MethodGet,
				pagePath(slug),
				url.Values{},
				func(resp pagePullResponse) error {
					return writePulledHTML(opts, slug, dest, resp.RenderedHTML)
				},
			)
			return translatePullError(err, slug)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Write the HTML to this path (- for stdout; defaults to <slug>.html)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite the output file if it already exists")

	return cmd
}

func pullArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return cmdutil.UsageErrorf(cmd, "missing page slug")
	}
	if len(args) > 1 {
		return cmdutil.UsageErrorf(cmd, "unexpected argument: %s", args[1])
	}
	return nil
}

func writePulledHTML(opts cmdutil.Options, slug, dest, html string) error {
	if html == "" {
		return fmt.Errorf("page %q returned no rendered HTML — nothing to pull", slug)
	}

	if dest == "-" {
		return output.Writef(opts.Out(), "%s", html)
	}

	if err := os.WriteFile(dest, []byte(html), 0644); err != nil { //nolint:gosec // G306: pulled pages are public storefront HTML, not secrets
		return err
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{slug, dest}})
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold(fmt.Sprintf("Pulled %s → %s", slug, dest))); err != nil {
		return err
	}
	return output.Writef(opts.Out(), "Edit it, check with `gumroad pages preview %s`, then publish with `gumroad pages push %s %s`.\n", dest, slug, dest)
}

func translatePullError(err error, slug string) error {
	if err == nil {
		return nil
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    fmt.Sprintf("page not found: %s", slug),
			Hint:       "Run `gumroad pages list` to see your pages.",
		}
	}
	return err
}
