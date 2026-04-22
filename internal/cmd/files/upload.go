package files

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

// s3HTTPClientForTesting redirects S3 part PUTs at a test server. Production
// leaves this nil so upload.Upload falls back to its shared client. Tests in
// this package must not use t.Parallel — overwriting this var across goroutines
// would race.
var s3HTTPClientForTesting *http.Client

func newUploadCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "upload <path>",
		Short: "Upload a file and print the canonical URL",
		Args:  cmdutil.ExactArgs(1),
		Example: `  gumroad files upload ./pack.zip
  gumroad files upload ./pack.zip --name "Art Pack.zip"
  gumroad files upload ./pack.zip --json --jq '.file_url'`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			path := args[0]

			plan, err := upload.Describe(path, upload.Options{Filename: name})
			if err != nil {
				return err
			}

			if opts.DryRun {
				return renderDryRun(opts, plan)
			}

			return runUpload(opts, path, plan)
		},
	}
	c.Flags().StringVar(&name, "name", "", "Override the display filename sent to the server (defaults to the basename of <path>)")
	return c
}

func runUpload(opts cmdutil.Options, path string, plan upload.Plan) error {
	token, err := config.Token()
	if err != nil {
		return err
	}
	client := cmdutil.NewAPIClient(opts, token)
	fileURL, err := uploadui.UploadFile(opts, client, path, plan, s3HTTPClientForTesting, plan.Filename)
	if err != nil {
		return err
	}
	return renderFileURL(opts, fileURL)
}

func renderFileURL(opts cmdutil.Options, fileURL string) error {
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(map[string]string{"file_url": fileURL})
		if err != nil {
			return fmt.Errorf("could not encode JSON output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{fileURL}})
	}
	return output.Writeln(opts.Out(), fileURL)
}

type dryRunUploadPlan struct {
	DryRun    bool   `json:"dry_run"`
	Action    string `json:"action"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	PartSize  int64  `json:"part_size"`
	PartCount int    `json:"part_count"`
}

func renderDryRun(opts cmdutil.Options, plan upload.Plan) error {
	payload := dryRunUploadPlan{
		DryRun:    true,
		Action:    "upload",
		Path:      plan.Path,
		Filename:  plan.Filename,
		Size:      plan.Size,
		PartSize:  plan.PartSize,
		PartCount: plan.PartCount,
	}
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			"upload",
			plan.Path,
			plan.Filename,
			strconv.FormatInt(plan.Size, 10),
			strconv.Itoa(plan.PartCount),
		}})
	}
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": upload "+plan.Path); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Filename: %s\n", plan.Filename); err != nil {
		return err
	}
	parts := "1 part"
	if plan.PartCount != 1 {
		parts = fmt.Sprintf("%d parts", plan.PartCount)
	}
	return output.Writef(opts.Out(), "Size: %s (%s)\n", uploadui.HumanBytes(plan.Size), parts)
}
