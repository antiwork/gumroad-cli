package variants

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/richcontent"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

// s3HTTPClientForTesting redirects multipart PUTs at a test TLS server. Tests
// in this package must not use t.Parallel while mutating this hook.
var s3HTTPClientForTesting *http.Client

type requestedVariantUpload struct {
	Path        string
	DisplayName string
	Description string
}

type plannedVariantUpload struct {
	requestedVariantUpload
	Plan upload.Plan
}

type variantExistingProductFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type variantProductFileState struct {
	Files                            []variantExistingProductFile `json:"files"`
	HasSameRichContentForAllVariants bool                         `json:"has_same_rich_content_for_all_variants"`
}

type variantProductFilesResponse struct {
	Product variantProductFileState `json:"product"`
}

type variantRichContentState struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	RichContent []map[string]any `json:"rich_content"`
}

type variantRichContentResponse struct {
	Variant variantRichContentState `json:"variant"`
}

type variantFileAttachRequest struct {
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body"`
}

type variantFileAttachDryRun struct {
	DryRun         bool                        `json:"dry_run"`
	Uploads        []variantDryRunUpload       `json:"uploads"`
	Preserved      []variantDryRunExistingFile `json:"preserved"`
	ProductRequest variantFileAttachRequest    `json:"product_request"`
	VariantRequest variantFileAttachRequest    `json:"variant_request"`
}

type variantDryRunUpload struct {
	Action    string `json:"action"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	PartSize  int64  `json:"part_size"`
	PartCount int    `json:"part_count"`
}

type variantDryRunExistingFile struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

func collectRequestedVariantUploads(cmd *cobra.Command, paths, names, descriptions []string) ([]requestedVariantUpload, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	alignedNames, err := alignVariantUploadValues(cmd, "--file-name", names, len(paths))
	if err != nil {
		return nil, err
	}
	alignedDescriptions, err := alignVariantUploadValues(cmd, "--file-description", descriptions, len(paths))
	if err != nil {
		return nil, err
	}

	uploads := make([]requestedVariantUpload, len(paths))
	for i, path := range paths {
		uploads[i] = requestedVariantUpload{
			Path:        path,
			DisplayName: strings.TrimSpace(alignedNames[i]),
			Description: alignedDescriptions[i],
		}
	}
	return uploads, nil
}

func alignVariantUploadValues(cmd *cobra.Command, flagName string, values []string, count int) ([]string, error) {
	switch len(values) {
	case 0:
		return make([]string, count), nil
	case count:
		aligned := make([]string, count)
		copy(aligned, values)
		return aligned, nil
	default:
		return nil, cmdutil.UsageErrorf(cmd, "%s must be provided zero times or exactly once per --file (got %d values for %d files)", flagName, len(values), count)
	}
}

func runVariantUpdateWithFiles(
	opts cmdutil.Options,
	productID, categoryID, variantID, variantPath string,
	params url.Values,
	requestedUploads []requestedVariantUpload,
) error {
	plannedUploads, err := describeVariantUploads(requestedUploads)
	if err != nil {
		return err
	}

	token, err := config.Token()
	if err != nil {
		return err
	}
	client := cmdutil.NewAPIClient(opts, token)

	productState, err := fetchVariantProductFileState(client, productID)
	if err != nil {
		return err
	}
	if productState.HasSameRichContentForAllVariants {
		return fmt.Errorf("cannot attach files to variant content while product %s uses shared content; use gumroad products update %s --file <path> instead, or switch the product to per-variant content first", productID, productID)
	}

	variantState, err := fetchVariantRichContentState(client, variantPath)
	if err != nil {
		return err
	}

	fileRefs, err := richcontent.NewFileRefs(len(plannedUploads))
	if err != nil {
		return err
	}
	richContent, err := richcontent.AppendFileEmbeds(variantState.RichContent, nil, fileRefs)
	if err != nil {
		return err
	}

	productPath := cmdutil.JoinPath("products", productID)
	productBody := map[string]any{
		"files": buildVariantProductFilesPayload(productState.Files, plannedUploads, placeholderVariantUploadURLs(len(plannedUploads)), fileRefs),
	}
	variantBody := buildVariantUpdateJSONBody(params, richContent)

	if opts.DryRun {
		return renderVariantFileAttachDryRun(opts, productPath, variantPath, productState.Files, plannedUploads, productBody, variantBody)
	}

	uploadedURLs, err := uploadVariantBatch(opts, client, plannedUploads)
	if err != nil {
		return err
	}
	productBody["files"] = buildVariantProductFilesPayload(productState.Files, plannedUploads, uploadedURLs, fileRefs)

	if _, err := client.PutJSON(productPath, productBody); err != nil {
		return wrapVariantPartialUploadError(err, uploadedURLs)
	}
	data, err := client.PutJSON(variantPath, variantBody)
	if err != nil {
		return wrapVariantPartialUploadError(err, uploadedURLs)
	}
	return cmdutil.PrintMutationSuccess(opts, data, variantID, "Variant "+variantID+" updated.")
}

func describeVariantUploads(uploads []requestedVariantUpload) ([]plannedVariantUpload, error) {
	planned := make([]plannedVariantUpload, len(uploads))
	for i, requested := range uploads {
		plan, err := upload.Describe(requested.Path, upload.Options{Filename: requested.DisplayName})
		if err != nil {
			return nil, err
		}

		planned[i] = plannedVariantUpload{
			requestedVariantUpload: requested,
			Plan:                   plan,
		}
	}
	return planned, nil
}

func fetchVariantProductFileState(client *api.Client, productID string) (variantProductFileState, error) {
	data, err := client.Get(cmdutil.JoinPath("products", productID), url.Values{})
	if err != nil {
		return variantProductFileState{}, err
	}
	resp, err := cmdutil.DecodeJSON[variantProductFilesResponse](data)
	if err != nil {
		return variantProductFileState{}, err
	}
	return resp.Product, nil
}

func fetchVariantRichContentState(client *api.Client, variantPath string) (variantRichContentState, error) {
	data, err := client.Get(variantPath, url.Values{})
	if err != nil {
		return variantRichContentState{}, err
	}
	resp, err := cmdutil.DecodeJSON[variantRichContentResponse](data)
	if err != nil {
		return variantRichContentState{}, err
	}
	return resp.Variant, nil
}

func buildVariantProductFilesPayload(
	existing []variantExistingProductFile,
	uploads []plannedVariantUpload,
	uploadURLs []string,
	fileRefs []richcontent.FileRef,
) []map[string]any {
	files := make([]map[string]any, 0, len(existing)+len(uploads))
	for _, file := range existing {
		files = append(files, map[string]any{"id": file.ID})
	}
	for i, planned := range uploads {
		entry := map[string]any{
			"external_id": fileRefs[i].FileID,
			"url":         uploadURLs[i],
		}
		if planned.DisplayName != "" {
			entry["display_name"] = planned.DisplayName
		}
		if planned.Description != "" {
			entry["description"] = planned.Description
		}
		files = append(files, entry)
	}
	return files
}

func buildVariantUpdateJSONBody(params url.Values, richContent []map[string]any) map[string]any {
	body := valuesToJSONBody(params)
	body["rich_content"] = richContent
	return body
}

func valuesToJSONBody(params url.Values) map[string]any {
	body := make(map[string]any, len(params))
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := append([]string(nil), params[key]...)
		if len(values) == 1 {
			body[key] = values[0]
		} else if len(values) > 1 {
			body[key] = values
		}
	}
	return body
}

func placeholderVariantUploadURLs(count int) []string {
	urls := make([]string, count)
	for i := 0; i < count; i++ {
		urls[i] = fmt.Sprintf("<uploaded:file:%d>", i)
	}
	return urls
}

func uploadVariantBatch(opts cmdutil.Options, client *api.Client, uploads []plannedVariantUpload) ([]string, error) {
	urls := make([]string, len(uploads))
	for i, current := range uploads {
		statusLabel := current.Plan.Filename
		if len(uploads) > 1 {
			statusLabel = fmt.Sprintf("%s (%d/%d)", current.Plan.Filename, i+1, len(uploads))
		}

		fileURL, err := uploadui.UploadFile(opts, client, current.Path, current.Plan, s3HTTPClientForTesting, statusLabel)
		if err != nil {
			return nil, wrapVariantPartialUploadError(err, urls[:i])
		}
		urls[i] = fileURL
	}
	return urls, nil
}

type variantPartialUploadError struct {
	cause        error
	uploadedURLs []string
}

func (e *variantPartialUploadError) Error() string {
	if len(e.uploadedURLs) == 0 {
		return e.cause.Error()
	}
	return fmt.Sprintf("%v (previously uploaded file URLs: %s)", e.cause, strings.Join(e.uploadedURLs, ", "))
}

func (e *variantPartialUploadError) Unwrap() error {
	return e.cause
}

func wrapVariantPartialUploadError(err error, uploadedURLs []string) error {
	if err == nil {
		return nil
	}
	if len(uploadedURLs) == 0 {
		return err
	}
	copied := append([]string(nil), uploadedURLs...)
	return &variantPartialUploadError{
		cause:        err,
		uploadedURLs: copied,
	}
}

func renderVariantFileAttachDryRun(
	opts cmdutil.Options,
	productPath, variantPath string,
	existingFiles []variantExistingProductFile,
	uploads []plannedVariantUpload,
	productBody, variantBody map[string]any,
) error {
	payload := variantFileAttachDryRun{
		DryRun:    true,
		Uploads:   make([]variantDryRunUpload, 0, len(uploads)),
		Preserved: make([]variantDryRunExistingFile, 0, len(existingFiles)),
		ProductRequest: variantFileAttachRequest{
			Method: http.MethodPut,
			Path:   productPath,
			Body:   productBody,
		},
		VariantRequest: variantFileAttachRequest{
			Method: http.MethodPut,
			Path:   variantPath,
			Body:   variantBody,
		},
	}
	for _, planned := range uploads {
		payload.Uploads = append(payload.Uploads, dryRunVariantUpload(planned.Plan))
	}
	for _, current := range existingFiles {
		payload.Preserved = append(payload.Preserved, variantDryRunExistingFile(current))
	}

	switch {
	case opts.UsesJSONOutput():
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	case opts.PlainOutput:
		return renderVariantFileAttachDryRunPlain(opts, payload)
	default:
		return renderVariantFileAttachDryRunHuman(opts, payload)
	}
}

func dryRunVariantUpload(plan upload.Plan) variantDryRunUpload {
	return variantDryRunUpload{
		Action:    "upload",
		Path:      plan.Path,
		Filename:  plan.Filename,
		Size:      plan.Size,
		PartSize:  plan.PartSize,
		PartCount: plan.PartCount,
	}
}

func renderVariantFileAttachDryRunPlain(opts cmdutil.Options, payload variantFileAttachDryRun) error {
	for _, current := range payload.Preserved {
		if err := output.PrintPlain(opts.Out(), [][]string{{"preserve", current.ID, current.Name}}); err != nil {
			return err
		}
	}
	for _, planned := range payload.Uploads {
		if err := output.PrintPlain(opts.Out(), [][]string{{
			"upload",
			planned.Path,
			planned.Filename,
			strconv.FormatInt(planned.Size, 10),
			strconv.Itoa(planned.PartCount),
		}}); err != nil {
			return err
		}
	}
	for _, request := range []variantFileAttachRequest{payload.ProductRequest, payload.VariantRequest} {
		data, err := json.Marshal(request.Body)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		if err := output.PrintPlain(opts.Out(), [][]string{{request.Method, request.Path, string(data)}}); err != nil {
			return err
		}
	}
	return nil
}

func renderVariantFileAttachDryRunHuman(opts cmdutil.Options, payload variantFileAttachDryRun) error {
	for _, current := range payload.Preserved {
		if err := output.Writeln(opts.Out(), "Preserve existing file: "+formatVariantExistingFileLabel(current)); err != nil {
			return err
		}
	}
	for _, planned := range payload.Uploads {
		if err := output.Writeln(opts.Out(), opts.Style().Yellow("Dry run")+": upload "+planned.Path); err != nil {
			return err
		}
		if err := output.Writef(opts.Out(), "Filename: %s\n", planned.Filename); err != nil {
			return err
		}
		parts := "1 part"
		if planned.PartCount != 1 {
			parts = fmt.Sprintf("%d parts", planned.PartCount)
		}
		if err := output.Writef(opts.Out(), "Size: %s (%s)\n", uploadui.HumanBytes(planned.Size), parts); err != nil {
			return err
		}
	}
	for _, request := range []variantFileAttachRequest{payload.ProductRequest, payload.VariantRequest} {
		if err := output.Writeln(opts.Out(), opts.Style().Yellow("Dry run")+": "+request.Method+" "+request.Path); err != nil {
			return err
		}
		data, err := json.MarshalIndent(request.Body, "", "  ")
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		if err := output.Writeln(opts.Out(), string(data)); err != nil {
			return err
		}
	}
	return nil
}

func formatVariantExistingFileLabel(file variantDryRunExistingFile) string {
	name := strings.TrimSpace(file.Name)
	switch {
	case name == "":
		return file.ID
	case name == file.ID:
		return name
	default:
		return fmt.Sprintf("%s (%s)", name, file.ID)
	}
}
