package pageutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
)

type Target struct {
	Resource    string
	ID          string
	Path        string
	PreviewPath string
}

type Product struct {
	CustomHTML     *string `json:"custom_html"`
	Description    *string `json:"description"`
	FormattedPrice string  `json:"formatted_price"`
	LandingURL     string  `json:"landing_url"`
	Name           string  `json:"name"`
}

type UpdateResponse struct {
	Success            bool               `json:"success"`
	Product            Product            `json:"product"`
	PreviousCustomHTML *string            `json:"previous_custom_html"`
	SanitizationReport SanitizationReport `json:"sanitization_report"`
}

type PreviewResponse struct {
	Success            bool               `json:"success"`
	CustomHTML         *string            `json:"custom_html"`
	SanitizationReport SanitizationReport `json:"sanitization_report"`
}

type ShowResponse struct {
	Success bool    `json:"success"`
	Product Product `json:"product"`
}

type pageHTMLPayload struct {
	CustomHTML any `json:"custom_html"`
}

func ProductTarget(id string) Target {
	return Target{
		Resource:    "products",
		ID:          id,
		Path:        cmdutil.JoinPath("products", id),
		PreviewPath: cmdutil.JoinPath("products", id, "preview_custom_html"),
	}
}

func Push(opts cmdutil.Options, target Target, path string) error {
	html, source, err := ReadHTML(opts, path)
	if err != nil {
		return err
	}
	if opts.DryRun {
		previewOpts := opts
		previewOpts.DryRun = false
		return previewHTML(previewOpts, target, source, html, false)
	}

	data, err := putCustomHTML(opts, target, &html, "Publishing page...")
	if err != nil {
		return err
	}
	resp, err := cmdutil.DecodeJSON[UpdateResponse](data)
	if err != nil {
		return err
	}
	snapshot := saveSnapshotAfterWrite(opts, target, resp.PreviousCustomHTML)
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	return renderPushResult(opts, target, source, html, resp, snapshot)
}

func Preview(opts cmdutil.Options, target Target, path string, diff bool) error {
	html, source, err := ReadHTML(opts, path)
	if err != nil {
		return err
	}
	return previewHTML(opts, target, source, html, diff)
}

func Clear(opts cmdutil.Options, target Target) error {
	ok, err := cmdutil.ConfirmAction(opts, "Clear custom HTML page for product "+target.ID+"?")
	if err != nil {
		return err
	}
	if !ok {
		return cmdutil.PrintCancelledAction(opts, "clear custom HTML page for product "+target.ID, target.ID)
	}
	if opts.DryRun {
		return cmdutil.PrintDryRunAction(opts, "clear custom HTML page for product "+target.ID)
	}

	data, err := putCustomHTML(opts, target, nil, "Clearing page...")
	if err != nil {
		return err
	}
	resp, err := cmdutil.DecodeJSON[UpdateResponse](data)
	if err != nil {
		return err
	}
	snapshot := saveSnapshotAfterWrite(opts, target, resp.PreviousCustomHTML)
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	return renderClearResult(opts, target, snapshot)
}

func Restore(opts cmdutil.Options, target Target, index int) error {
	snapshot, html, err := ReadSnapshot(target.Resource, target.ID, index)
	if err != nil {
		return err
	}
	ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf("Restore product %s from snapshot %d (%s, %d bytes)?", target.ID, snapshot.Index, snapshot.Timestamp.Format(timeStampForPrompt), snapshot.Size))
	if err != nil {
		return err
	}
	if !ok {
		return cmdutil.PrintCancelledAction(opts, "restore product page from snapshot", target.ID)
	}
	if opts.DryRun {
		return cmdutil.PrintDryRunAction(opts, fmt.Sprintf("restore product %s from %s", target.ID, snapshot.Path))
	}

	data, err := putCustomHTML(opts, target, &html, "Restoring page...")
	if err != nil {
		return err
	}
	resp, err := cmdutil.DecodeJSON[UpdateResponse](data)
	if err != nil {
		return err
	}
	saved := saveSnapshotAfterWrite(opts, target, resp.PreviousCustomHTML)
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	return renderRestoreResult(opts, target, snapshot, html, resp, saved)
}

func History(opts cmdutil.Options, target Target) error {
	snapshots, err := ListSnapshots(target.Resource, target.ID)
	if err != nil {
		return err
	}
	return RenderHistory(opts, target.Resource, target.ID, snapshots)
}

func URL(opts cmdutil.Options, target Target) error {
	data, err := getProduct(opts, target, "Fetching page URL...")
	if err != nil {
		return err
	}
	resp, err := cmdutil.DecodeJSON[ShowResponse](data)
	if err != nil {
		return err
	}
	if resp.Product.LandingURL == "" {
		return fmt.Errorf("product response did not include landing_url")
	}
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{resp.Product.LandingURL}})
	}
	return output.Writeln(opts.Out(), resp.Product.LandingURL)
}

const timeStampForPrompt = "2006-01-02 15:04:05 UTC"

func previewHTML(opts cmdutil.Options, target Target, source string, html string, diff bool) error {
	var current *string
	if diff && !opts.UsesJSONOutput() {
		data, err := getProduct(opts, target, "Fetching current page...")
		if err != nil {
			return err
		}
		resp, err := cmdutil.DecodeJSON[ShowResponse](data)
		if err != nil {
			return err
		}
		current = resp.Product.CustomHTML
	}

	data, err := postPreview(opts, target, html, "Previewing page...")
	if err != nil {
		return err
	}
	resp, err := cmdutil.DecodeJSON[PreviewResponse](data)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	if err := renderPreviewResult(opts, source, html, resp); err != nil {
		return err
	}
	if diff && !opts.PlainOutput && !opts.Quiet {
		return RenderUnifiedDiff(opts.Out(), "current", current, "preview", resp.CustomHTML)
	}
	return nil
}

func putCustomHTML(opts cmdutil.Options, target Target, html *string, spinnerMessage string) (json.RawMessage, error) {
	payload := pageHTMLPayload{}
	if html != nil {
		payload.CustomHTML = *html
	}
	return runPageData(opts, spinnerMessage, func(client *api.Client) (json.RawMessage, error) {
		data, err := client.PutJSON(target.Path, payload)
		return data, withRateLimitHint(err, "30 PUTs/min per token", "Use `gumroad products page preview "+target.ID+"` to iterate without burning your push budget.")
	})
}

func postPreview(opts cmdutil.Options, target Target, html string, spinnerMessage string) (json.RawMessage, error) {
	payload := pageHTMLPayload{CustomHTML: html}
	return runPageData(opts, spinnerMessage, func(client *api.Client) (json.RawMessage, error) {
		data, err := client.PostJSON(target.PreviewPath, payload)
		return data, withRateLimitHint(err, "60 previews/min per token", "Pause briefly before previewing again.")
	})
}

func getProduct(opts cmdutil.Options, target Target, spinnerMessage string) (json.RawMessage, error) {
	return runPageData(opts, spinnerMessage, func(client *api.Client) (json.RawMessage, error) {
		return client.Get(target.Path, url.Values{})
	})
}

func runPageData(opts cmdutil.Options, spinnerMessage string, run func(*api.Client) (json.RawMessage, error)) (json.RawMessage, error) {
	token, err := config.Token()
	if err != nil {
		return nil, err
	}
	if cmdutil.ShouldShowSpinner(opts) {
		sp := output.NewSpinnerTo(spinnerMessage, opts.Err())
		sp.Start()
		defer sp.Stop()
	}
	client := cmdutil.NewAPIClient(opts, token)
	return run(client)
}

func withRateLimitHint(err error, limit string, hint string) error {
	if err == nil {
		return nil
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    fmt.Sprintf("Hit Gumroad's rate limit (%s).", limit),
			Hint:       hint,
		}
	}
	return err
}

func saveSnapshotAfterWrite(opts cmdutil.Options, target Target, html *string) SavedSnapshot {
	snapshot, err := SaveSnapshot(target.Resource, target.ID, html)
	if err != nil {
		if !opts.Quiet {
			_, _ = fmt.Fprintf(opts.Err(), "Warning: page was updated, but the previous HTML snapshot could not be saved: %v\n", err)
		}
		return SavedSnapshot{}
	}
	return snapshot
}

func renderPushResult(opts cmdutil.Options, target Target, source string, html string, resp UpdateResponse, snapshot SavedSnapshot) error {
	if opts.PlainOutput {
		rows := resultPlainRows("pushed", target, source, html, resp.Product.CustomHTML, resp.Product.LandingURL, resp.SanitizationReport, snapshot)
		return output.PrintPlain(opts.Out(), rows)
	}
	if err := RenderReport(opts, source, html, resp.Product.CustomHTML, resp.SanitizationReport); err != nil {
		return err
	}
	if err := cmdutil.PrintSuccess(opts, "Page pushed to product "+target.ID+"."); err != nil {
		return err
	}
	return renderLiveAndSnapshot(opts, target, resp.Product.LandingURL, snapshot)
}

func renderPreviewResult(opts cmdutil.Options, source string, html string, resp PreviewResponse) error {
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), append([][]string{{"preview"}}, ReportPlainRows(html, resp.CustomHTML, resp.SanitizationReport)...))
	}
	if err := RenderReport(opts, source, html, resp.CustomHTML, resp.SanitizationReport); err != nil {
		return err
	}
	return cmdutil.PrintInfo(opts, "Preview only; nothing was published.")
}

func renderClearResult(opts cmdutil.Options, target Target, snapshot SavedSnapshot) error {
	if opts.PlainOutput {
		rows := [][]string{{"cleared", target.ID}}
		if snapshot.Written {
			rows = append(rows, []string{"snapshot", snapshot.Path, fmt.Sprintf("%d", snapshot.Size)})
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if err := cmdutil.PrintSuccess(opts, "Page cleared for product "+target.ID+"."); err != nil {
		return err
	}
	return renderSnapshotHint(opts, target, snapshot)
}

func renderRestoreResult(opts cmdutil.Options, target Target, snapshot Snapshot, html string, resp UpdateResponse, saved SavedSnapshot) error {
	if opts.PlainOutput {
		rows := resultPlainRows("restored", target, snapshot.Path, html, resp.Product.CustomHTML, resp.Product.LandingURL, resp.SanitizationReport, saved)
		return output.PrintPlain(opts.Out(), rows)
	}
	if err := RenderReport(opts, snapshot.Path, html, resp.Product.CustomHTML, resp.SanitizationReport); err != nil {
		return err
	}
	if err := cmdutil.PrintSuccess(opts, fmt.Sprintf("Page restored for product %s from snapshot %d.", target.ID, snapshot.Index)); err != nil {
		return err
	}
	return renderLiveAndSnapshot(opts, target, resp.Product.LandingURL, saved)
}

func renderLiveAndSnapshot(opts cmdutil.Options, target Target, landingURL string, snapshot SavedSnapshot) error {
	if opts.Quiet {
		return nil
	}
	if landingURL != "" {
		if err := output.Writeln(opts.Out(), "Live at "+landingURL); err != nil {
			return err
		}
	}
	return renderSnapshotHint(opts, target, snapshot)
}

func renderSnapshotHint(opts cmdutil.Options, target Target, snapshot SavedSnapshot) error {
	if opts.Quiet || !snapshot.Written {
		return nil
	}
	if err := output.Writeln(opts.Out(), "Snapshot saved: "+snapshot.Path); err != nil {
		return err
	}
	return output.Writeln(opts.Out(), "Restore with: gumroad products page restore "+target.ID)
}

func resultPlainRows(action string, target Target, source string, original string, sanitized *string, landingURL string, report SanitizationReport, snapshot SavedSnapshot) [][]string {
	originalBytes, sanitizedBytes, delta := reportSizes(original, sanitized)
	rows := [][]string{{
		action,
		target.ID,
		landingURL,
		source,
		fmt.Sprintf("%d", originalBytes),
		fmt.Sprintf("%d", sanitizedBytes),
		fmt.Sprintf("%d", delta),
		fmt.Sprintf("%d", report.TotalRemoved),
		fmt.Sprintf("%t", report.Truncated),
	}}
	if snapshot.Written {
		rows = append(rows, []string{"snapshot", snapshot.Path, fmt.Sprintf("%d", snapshot.Size)})
	}
	rows = append(rows, ReportPlainRows(original, sanitized, report)[1:]...)
	return rows
}
