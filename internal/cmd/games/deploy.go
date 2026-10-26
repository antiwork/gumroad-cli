package games

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

// s3HTTPClientForTesting redirects S3 part PUTs at a test server; nil in production.
var s3HTTPClientForTesting *http.Client

func newDeployCmd() *cobra.Command {
	var product string
	var promote bool
	c := &cobra.Command{
		Use:   "deploy <dir|zip>",
		Short: "Upload a game build and record it as a new version",
		Long: "Upload a game build and record it as a new immutable version of the product's game.\n\n" +
			"<dir|zip> is a .zip, or a folder with index.html at its top level (zipped for you; dotfiles are skipped). " +
			"The build is not live until promoted — pass --promote to do both.",
		Args: cmdutil.ExactArgs(1),
		Example: `  gumroad games deploy ./dist --product abc123
  gumroad games deploy ./game.zip --product abc123 --promote --json`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := args[0]
			if strings.TrimSpace(product) == "" {
				return cmdutil.UsageErrorf(c, "--product is required")
			}

			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("could not stat %s: %w", path, err)
			}
			if !info.IsDir() && !strings.EqualFold(filepath.Ext(path), ".zip") {
				return fmt.Errorf("%s is not a .zip or a directory", path)
			}
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "deploy "+path+" to product "+product+promoteSuffix(promote))
			}

			zipPath := path
			if info.IsDir() {
				if zipPath, err = zipDir(path); err != nil {
					return err
				}
				defer func() { _ = os.Remove(zipPath) }()
			}

			plan, err := upload.Describe(zipPath, upload.Options{Filename: "game-build.zip"})
			if err != nil {
				return err
			}
			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)
			fileURL, err := uploadui.UploadFile(opts, client, zipPath, plan, s3HTTPClientForTesting, plan.Filename)
			if err != nil {
				return err
			}

			data, err := cmdutil.RunRequestWithTokenData(opts, token, "Deploying build...", http.MethodPost,
				buildsPath(product, "deploy"), url.Values{"url": {fileURL}})
			if err != nil {
				return err
			}
			resp, err := cmdutil.DecodeJSON[buildResponse](data)
			if err != nil {
				return err
			}

			if promote {
				data, err = cmdutil.RunRequestWithTokenData(opts, token, "Promoting build...", http.MethodPost,
					buildsPath(product, resp.Build.ID, "promote"), url.Values{})
				if err != nil {
					return fmt.Errorf("build %d was deployed (id %s) but promoting it failed; run `gumroad games promote %s --product %s`: %w",
						resp.Build.Number, resp.Build.ID, resp.Build.ID, product, err)
				}
				if resp, err = cmdutil.DecodeJSON[buildResponse](data); err != nil {
					return err
				}
			}

			msg := fmt.Sprintf("Build %d (%s) is %s.", resp.Build.Number, resp.Build.ID, resp.Build.Status)
			return cmdutil.PrintMutationSuccess(opts, data, resp.Build.ID, msg)
		},
	}
	c.Flags().StringVar(&product, "product", "", "Product ID or permalink (required)")
	c.Flags().BoolVar(&promote, "promote", false, "Make the new build live right after deploying")
	return c
}

func promoteSuffix(promote bool) string {
	if promote {
		return " and promote it"
	}
	return ""
}
