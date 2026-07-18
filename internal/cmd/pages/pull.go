package pages

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

// Page slugs are lowercase letters, numbers, and hyphens (mirrors the
// backend's Page model validation). "profile" is the one special slug the
// pages commands accept on top of that.
var pageSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func newPullCmd() *cobra.Command {
	var outputPath string
	var force bool

	cmd := &cobra.Command{
		Use:   "pull <slug>",
		Short: "Download a page's custom HTML",
		Long: "Download the custom HTML a storefront page currently serves into a local file, so pull → edit → push is a real round trip: what you pull is exactly what was pushed.\n\n" +
			"Only pages that already have custom HTML can be pulled. For a rich-text page, or a profile without published custom HTML, use `gumroad pages scaffold` instead — it generates starter HTML from the page's current render.\n\n" +
			"Edit the file, check the result with `gumroad pages preview <file>`, then publish it with `gumroad pages push <slug> <file>`.\n\n" +
			"Use the slug \"profile\" to pull your profile landing page's published custom HTML.",
		Args: pullArgs,
		Example: `  gumroad pages pull about
  gumroad pages pull about -o custom.html
  gumroad pages pull about -o -
  gumroad pages pull profile
  gumroad pages pull about --json --jq '.page.title'`,
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
					if resp.CustomHTML == "" {
						return "", cmdutil.InvalidInputErrorf("your profile has no published custom HTML to pull — run `gumroad pages scaffold profile` to generate starter HTML from the current storefront render")
					}
					return resp.CustomHTML, nil
				}, dest, force, renderPullSuccess(opts, slug, dest))
				return translatePullError(err, slug)
			}

			err := runPullRequest(opts, pagePath(slug), func(data json.RawMessage) (string, error) {
				resp, err := cmdutil.DecodeJSON[pagePullResponse](data)
				if err != nil {
					return "", err
				}
				if resp.Page.CustomHTML == nil || *resp.Page.CustomHTML == "" {
					return "", cmdutil.InvalidInputErrorf("page %q is a rich-text page with no custom HTML to pull — run `gumroad pages scaffold %s` to generate starter HTML from its current render", slug, slug)
				}
				return *resp.Page.CustomHTML, nil
			}, dest, force, renderPullSuccess(opts, slug, dest))
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

// validatePullInvocation rejects invalid slugs and flag combinations before
// any file or network work happens.
func validatePullInvocation(opts cmdutil.Options, slug, outputPath string) error {
	if slug != profileSlug && !pageSlugPattern.MatchString(slug) {
		return cmdutil.InvalidInputErrorf("invalid page slug: %q (slugs contain only lowercase letters, numbers, and hyphens)", slug)
	}
	// JSON output owns stdout, so it cannot be combined with streaming the
	// HTML there. Rejecting the combination keeps every accepted invocation
	// an actual pull — output flags never silently cancel the download.
	if outputPath == "-" && opts.UsesJSONOutput() {
		return cmdutil.InvalidInputErrorf("--json/--jq cannot be combined with -o - (both write to stdout); use -o <path> to keep the file")
	}
	return nil
}

func pullDestination(slug, outputPath string) string {
	if outputPath != "" {
		return outputPath
	}
	return slug + ".html"
}

// checkDestinationWritable fails fast when the destination already exists and
// --force wasn't given, so no API call is wasted. The check runs again when
// the file is actually replaced (see writePulledFile) — this one is only an
// early exit.
func checkDestinationWritable(dest string, force bool) error {
	if dest == "-" || force {
		return nil
	}
	if _, err := os.Stat(dest); err == nil {
		return destinationExistsError(dest)
	}
	return nil
}

func destinationExistsError(dest string) error {
	return cmdutil.InvalidInputErrorf("%s already exists (use --force to overwrite, or -o to pick another path)", dest)
}

// runPullRequest fetches the page and hands the extracted HTML to the shared
// write path. JSON mode is handled here rather than by the shared JSON
// fast-path so that --json/--jq still perform the pull: the file is written
// first, then the raw response is printed.
func runPullRequest(
	opts cmdutil.Options,
	path string,
	extract func(json.RawMessage) (string, error),
	dest string,
	force bool,
	renderSuccess func() error,
) error {
	fetchOpts := opts
	fetchOpts.JSONOutput = false
	fetchOpts.JQExpr = ""

	return cmdutil.RunRequest(fetchOpts, "Pulling page...", http.MethodGet, path, url.Values{}, func(data json.RawMessage) error {
		html, err := extract(data)
		if err != nil {
			return err
		}
		if err := writePulledFile(opts, dest, html, force); err != nil {
			return err
		}
		if opts.UsesJSONOutput() {
			return cmdutil.PrintJSONResponse(opts, data)
		}
		return renderSuccess()
	})
}

// writePulledFile writes the HTML to dest via a temporary file in the same
// directory, moving it into place only after the whole download has been
// written. The destination is never touched on a failed or partial write, and
// --force is enforced at the moment the file is actually replaced.
func writePulledFile(opts cmdutil.Options, dest, html string, force bool) error {
	if dest == "-" {
		return output.Writef(opts.Out(), "%s", html)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".gumroad-pull-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(html); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Pulled pages are public storefront HTML, not secrets — match the
	// world-readable mode the rest of the CLI uses for non-secret content.
	if err := os.Chmod(tmpName, 0644); err != nil { //nolint:gosec // G302: public storefront HTML
		os.Remove(tmpName)
		return err
	}
	if !force {
		if _, err := os.Stat(dest); err == nil {
			os.Remove(tmpName)
			return destinationExistsError(dest)
		}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func renderPullSuccess(opts cmdutil.Options, slug, dest string) func() error {
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
		if err := output.Writeln(opts.Out(), style.Bold(fmt.Sprintf("Pulled %s → %s", slug, dest))); err != nil {
			return err
		}
		return output.Writef(opts.Out(), "Edit it, check with `gumroad pages preview %s`, then publish with `gumroad pages push %s %s`.\n", shellQuotePath(dest), slug, shellQuotePath(dest))
	}
}

// shellQuotePath makes a path safe to copy-paste into the suggested follow-up
// commands. Plain paths pass through untouched; anything with spaces or shell
// metacharacters gets single-quoted.
func shellQuotePath(path string) string {
	if plainShellPathPattern.MatchString(path) {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

var plainShellPathPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// translatePullError rewrites the API's missing-page response into a compact
// error with a discovery hint. The API reports a missing page as HTTP 200 with
// success: false and a "was not found" message (a plain 404 covers
// routing-level misses).
func translatePullError(err error, slug string) error {
	if err == nil {
		return nil
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusNotFound || strings.Contains(strings.ToLower(apiErr.Message), "not found") {
			return &api.APIError{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("page not found: %s", slug),
				Hint:       "Run `gumroad pages list` to see your pages.",
			}
		}
	}
	return err
}
