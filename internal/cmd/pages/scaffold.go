package pages

import (
	"encoding/json"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/pageutil"
	"github.com/spf13/cobra"
)

func newScaffoldCmd() *cobra.Command {
	var outputPath string
	var force bool

	cmd := &cobra.Command{
		Use:   "scaffold <slug>",
		Short: "Generate starter custom HTML from a page's current render",
		Long: "Generate starter custom HTML for a page that doesn't have any yet, using a static snapshot of how the page renders today.\n\n" +
			"The snapshot is a one-time export, not a faithful copy: pushing it converts the page to custom HTML, which replaces the dynamic storefront/editor experience — a rich-text page stops being editable in the in-app editor, and a default profile page stops updating automatically as your store changes.\n\n" +
			"For a page that already has custom HTML, use `gumroad pages pull` instead — that's the real pull → edit → push round trip.\n\n" +
			"Use the slug \"profile\" to scaffold from your profile landing page's default storefront render.",
		Args: pullArgs,
		Example: `  gumroad pages scaffold about
  gumroad pages scaffold about -o starter.html
  gumroad pages scaffold profile`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			slug := args[0]
			if err := validatePullInvocation(opts, slug, outputPath); err != nil {
				return err
			}
			dest := pullDestination(slug, outputPath)
			if err := checkDestinationWritable(dest, force); err != nil {
				return err
			}

			if slug == profileSlug {
				err := runPullRequest(opts, pageutil.ProfileTarget().Path, func(data json.RawMessage) (string, error) {
					resp, err := cmdutil.DecodeJSON[profilePullResponse](data)
					if err != nil {
						return "", err
					}
					if resp.CustomHTML != "" {
						return "", cmdutil.InvalidInputErrorf("your profile already has published custom HTML — use `gumroad pages pull profile` to download it")
					}
					if resp.RenderedHTML == "" {
						return "", cmdutil.InvalidInputErrorf("the API returned no render for your profile — nothing to scaffold from")
					}
					return resp.RenderedHTML, nil
				}, dest, force, renderScaffoldSuccess(opts, slug, dest))
				return translatePullError(err, slug)
			}

			err := runPullRequest(opts, pagePath(slug), func(data json.RawMessage) (string, error) {
				resp, err := cmdutil.DecodeJSON[pagePullResponse](data)
				if err != nil {
					return "", err
				}
				if resp.Page.CustomHTML != nil && *resp.Page.CustomHTML != "" {
					return "", cmdutil.InvalidInputErrorf("page %q already has custom HTML — use `gumroad pages pull %s` to download it", slug, slug)
				}
				if resp.RenderedHTML == "" {
					return "", cmdutil.InvalidInputErrorf("the API returned no render for page %q — nothing to scaffold from", slug)
				}
				return resp.RenderedHTML, nil
			}, dest, force, renderScaffoldSuccess(opts, slug, dest))
			return translatePullError(err, slug)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Write the HTML to this path (- for stdout; defaults to <slug>.html)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite the output file if it already exists")

	return cmd
}

func renderScaffoldSuccess(opts cmdutil.Options, slug, dest string) func() error {
	return func() error {
		if dest == "-" {
			return nil
		}
		if opts.PlainOutput {
			return output.PrintPlain(opts.Out(), [][]string{{slug, dest}})
		}
		if opts.Quiet {
			return nil
		}

		style := opts.Style()
		if err := output.Writeln(opts.Out(), style.Bold("Scaffolded "+slug+" → "+dest)); err != nil {
			return err
		}
		if err := output.Writef(opts.Out(), "This is a static snapshot, not a faithful copy. Pushing it converts %s to custom HTML and replaces the dynamic storefront/editor experience.\n", slug); err != nil {
			return err
		}
		return output.Writef(opts.Out(), "Edit it, check with `gumroad pages preview %s`, then publish with `gumroad pages push %s %s`.\n", shellQuotePath(dest), slug, shellQuotePath(dest))
	}
}
