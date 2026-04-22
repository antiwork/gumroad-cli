package uploadui

import (
	"fmt"
	"net/http"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/upload"
)

// UploadFile runs one multipart upload while keeping progress presentation in
// the command layer.
func UploadFile(opts cmdutil.Options, client *api.Client, path string, plan upload.Plan, httpClient *http.Client, statusLabel string) (string, error) {
	if statusLabel == "" {
		statusLabel = plan.Filename
	}

	totalLabel := HumanBytes(plan.Size)
	var sp *output.Spinner
	if cmdutil.ShouldShowSpinner(opts) {
		sp = output.NewSpinnerTo(uploadSpinnerStatus(statusLabel, 0, totalLabel), opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	progress := func(uploaded int64) {
		if sp != nil {
			sp.SetMessage(uploadSpinnerStatus(statusLabel, uploaded, totalLabel))
		}
	}

	fileURL, err := upload.Upload(opts.Context, client, path, upload.Options{
		Filename:   plan.Filename,
		HTTPClient: httpClient,
		Progress:   progress,
	})
	if err != nil {
		return "", err
	}
	if sp != nil {
		// Drain the spinner before the caller renders stdout.
		sp.Stop()
	}
	return fileURL, nil
}

func uploadSpinnerStatus(label string, uploaded int64, totalLabel string) string {
	return fmt.Sprintf("Uploading %s %s / %s", label, HumanBytes(uploaded), totalLabel)
}

// HumanBytes renders a byte count with IEC-style (1024-based) magnitudes but
// decimal-unit labels (KB/MB/GB), matching how Gumroad quotes file sizes.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", v, units[exp])
}
