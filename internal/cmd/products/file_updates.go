package products

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
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/spf13/cobra"
)

type requestedProductUpload struct {
	Path        string
	DisplayName string
	Description string
}

type plannedProductUpload struct {
	requestedProductUpload
	Plan upload.Plan
}

type existingProductFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type productFileUpdatePlan struct {
	Existing  []existingProductFile
	Preserved []existingProductFile
	Removed   []existingProductFile
	Uploads   []requestedProductUpload
}

type productFilesResponse struct {
	Product struct {
		Files []existingProductFile `json:"files"`
	} `json:"product"`
}

type dryRunUpdateBody struct {
	DryRun    bool                 `json:"dry_run"`
	Uploads   []dryRunCreateUpload `json:"uploads"`
	Preserved []dryRunExistingFile `json:"preserved"`
	Removed   []dryRunExistingFile `json:"removed"`
	Request   dryRunCreateRequest  `json:"request"`
}

type dryRunExistingFile struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

func collectRequestedProductUploads(
	cmd *cobra.Command,
	paths, names, descriptions []string,
) ([]requestedProductUpload, error) {
	if len(paths) == 0 {
		if len(names) > 0 {
			return nil, cmdutil.UsageErrorf(cmd, "--file-name requires at least one --file")
		}
		if len(descriptions) > 0 {
			return nil, cmdutil.UsageErrorf(cmd, "--file-description requires at least one --file")
		}
		return nil, nil
	}

	alignedNames, err := alignCreateUploadValues(cmd, "--file-name", names, len(paths))
	if err != nil {
		return nil, err
	}
	alignedDescriptions, err := alignCreateUploadValues(cmd, "--file-description", descriptions, len(paths))
	if err != nil {
		return nil, err
	}

	uploads := make([]requestedProductUpload, len(paths))
	for i, path := range paths {
		uploadSpec := requestedProductUpload{Path: path}
		uploadSpec.DisplayName = strings.TrimSpace(alignedNames[i])
		uploadSpec.Description = alignedDescriptions[i]
		uploads[i] = uploadSpec
	}
	return uploads, nil
}

func fetchExistingProductFiles(client *api.Client, productID string) ([]existingProductFile, error) {
	data, err := client.Get(cmdutil.JoinPath("products", productID), url.Values{})
	if err != nil {
		return nil, err
	}

	resp, err := cmdutil.DecodeJSON[productFilesResponse](data)
	if err != nil {
		return nil, err
	}
	return resp.Product.Files, nil
}

func planProductFileUpdate(
	cmd *cobra.Command,
	existing []existingProductFile,
	uploads []requestedProductUpload,
	keepIDs, removeIDs []string,
	replaceFiles bool,
) (productFileUpdatePlan, error) {
	keepSet, removeSet, err := validateProductFileSelections(cmd, keepIDs, removeIDs, replaceFiles)
	if err != nil {
		return productFileUpdatePlan{}, err
	}

	existingByID := make(map[string]existingProductFile, len(existing))
	for _, file := range existing {
		existingByID[file.ID] = file
	}

	if err := ensureKnownFileIDs(cmd, "--keep-file", keepSet, existingByID); err != nil {
		return productFileUpdatePlan{}, err
	}
	if err := ensureKnownFileIDs(cmd, "--remove-file", removeSet, existingByID); err != nil {
		return productFileUpdatePlan{}, err
	}

	plan := productFileUpdatePlan{
		Existing: existing,
		Uploads:  uploads,
	}

	for _, file := range existing {
		_, explicitlyRemoved := removeSet[file.ID]
		preserve := !replaceFiles
		if replaceFiles {
			_, preserve = keepSet[file.ID]
		}
		if explicitlyRemoved {
			preserve = false
		}

		if preserve {
			plan.Preserved = append(plan.Preserved, file)
		} else {
			plan.Removed = append(plan.Removed, file)
		}
	}

	return plan, nil
}

func ensureKnownFileIDs(
	cmd *cobra.Command,
	flagName string,
	requested map[string]struct{},
	existing map[string]existingProductFile,
) error {
	if len(requested) == 0 {
		return nil
	}

	var unknown []string
	for id := range requested {
		if _, ok := existing[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	sort.Strings(unknown)
	return cmdutil.UsageErrorf(cmd, "unknown %s id(s): %s", flagName, joinComma(unknown))
}

func describeProductUploads(uploads []requestedProductUpload) ([]plannedProductUpload, error) {
	planned := make([]plannedProductUpload, len(uploads))
	for i, requested := range uploads {
		plan, err := upload.Describe(requested.Path, upload.Options{Filename: requested.DisplayName})
		if err != nil {
			return nil, err
		}

		planned[i] = plannedProductUpload{
			requestedProductUpload: requested,
			Plan:                   plan,
		}
	}
	return planned, nil
}

func validateProductFileSelections(
	cmd *cobra.Command,
	keepIDs, removeIDs []string,
	replaceFiles bool,
) (map[string]struct{}, map[string]struct{}, error) {
	if len(keepIDs) > 0 && !replaceFiles {
		return nil, nil, cmdutil.UsageErrorf(cmd,
			"--keep-file can only be used together with --replace-files")
	}

	keepSet := make(map[string]struct{}, len(keepIDs))
	for _, id := range keepIDs {
		keepSet[id] = struct{}{}
	}
	removeSet := make(map[string]struct{}, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[id] = struct{}{}
	}

	var conflicts []string
	for id := range keepSet {
		if _, ok := removeSet[id]; ok {
			conflicts = append(conflicts, id)
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return nil, nil, cmdutil.UsageErrorf(cmd,
			"cannot use --keep-file and --remove-file for the same id(s): %s",
			joinComma(conflicts))
	}

	return keepSet, removeSet, nil
}

func buildProductUpdateFilesPayload(plan productFileUpdatePlan, uploadURLs []string) []map[string]any {
	files := make([]map[string]any, 0, len(plan.Preserved)+len(plan.Uploads))
	for _, file := range plan.Preserved {
		files = append(files, map[string]any{"id": file.ID})
	}
	for i, requested := range plan.Uploads {
		entry := map[string]any{"url": uploadURLs[i]}
		if requested.DisplayName != "" {
			entry["display_name"] = requested.DisplayName
		}
		if requested.Description != "" {
			entry["description"] = requested.Description
		}
		files = append(files, entry)
	}
	return files
}

func placeholderUploadURLs(count int) []string {
	urls := make([]string, count)
	for i := 0; i < count; i++ {
		urls[i] = fmt.Sprintf("<uploaded:file:%d>", i)
	}
	return urls
}

func renderProductUpdateDryRun(
	opts cmdutil.Options,
	path string,
	plan productFileUpdatePlan,
	uploads []plannedProductUpload,
	body map[string]any,
) error {
	switch {
	case opts.UsesJSONOutput():
		return renderProductUpdateDryRunJSON(opts, path, plan, uploads, body)
	case opts.PlainOutput:
		return renderProductUpdateDryRunPlain(opts, path, plan, uploads, body)
	default:
		return renderProductUpdateDryRunHuman(opts, path, plan, uploads, body)
	}
}

func renderProductUpdateDryRunJSON(
	opts cmdutil.Options,
	path string,
	plan productFileUpdatePlan,
	uploads []plannedProductUpload,
	body map[string]any,
) error {
	payload := dryRunUpdateBody{
		DryRun:    true,
		Uploads:   make([]dryRunCreateUpload, 0, len(uploads)),
		Preserved: make([]dryRunExistingFile, 0, len(plan.Preserved)),
		Removed:   make([]dryRunExistingFile, 0, len(plan.Removed)),
		Request: dryRunCreateRequest{
			Method: http.MethodPut,
			Path:   path,
			Body:   body,
		},
	}
	for _, planned := range uploads {
		payload.Uploads = append(payload.Uploads, dryRunProductUpload(planned.Plan))
	}
	for _, current := range plan.Preserved {
		payload.Preserved = append(payload.Preserved, dryRunExistingProductFile(current))
	}
	for _, current := range plan.Removed {
		payload.Removed = append(payload.Removed, dryRunExistingProductFile(current))
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not encode dry-run output: %w", err)
	}
	return output.PrintJSON(opts.Out(), data, opts.JQExpr)
}

func renderProductUpdateDryRunPlain(
	opts cmdutil.Options,
	path string,
	plan productFileUpdatePlan,
	uploads []plannedProductUpload,
	body map[string]any,
) error {
	for _, current := range plan.Preserved {
		if err := output.PrintPlain(opts.Out(), [][]string{{
			"preserve",
			current.ID,
			current.Name,
		}}); err != nil {
			return err
		}
	}
	for _, current := range plan.Removed {
		if err := output.PrintPlain(opts.Out(), [][]string{{
			"remove",
			current.ID,
			current.Name,
		}}); err != nil {
			return err
		}
	}
	for _, planned := range uploads {
		if err := renderProductUploadDryRunPlain(opts, planned.Plan); err != nil {
			return err
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("could not encode dry-run output: %w", err)
	}
	return output.PrintPlain(opts.Out(), [][]string{{
		http.MethodPut,
		path,
		string(data),
	}})
}

func renderProductUpdateDryRunHuman(
	opts cmdutil.Options,
	path string,
	plan productFileUpdatePlan,
	uploads []plannedProductUpload,
	body map[string]any,
) error {
	for _, current := range plan.Preserved {
		if err := output.Writeln(opts.Out(), "Preserve existing file: "+formatExistingProductFileLabel(current)); err != nil {
			return err
		}
	}
	for _, current := range plan.Removed {
		if err := output.Writeln(opts.Out(), "Remove existing file: "+formatExistingProductFileLabel(current)); err != nil {
			return err
		}
	}
	for _, planned := range uploads {
		if err := renderProductUploadDryRun(opts, planned.Plan); err != nil {
			return err
		}
	}
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": "+http.MethodPut+" "+path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode dry-run output: %w", err)
	}
	return output.Writeln(opts.Out(), string(data))
}

func runProductUpdateJSON(
	opts cmdutil.Options,
	client *api.Client,
	path, productID string,
	body map[string]any,
) error {
	var sp *output.Spinner
	if cmdutil.ShouldShowSpinner(opts) {
		sp = output.NewSpinnerTo("Updating product...", opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	data, err := client.PutJSON(path, body)
	if err != nil {
		return err
	}
	if sp != nil {
		sp.Stop()
	}
	return cmdutil.PrintMutationSuccess(opts, data, productID, "Product "+productID+" updated.")
}

func confirmProductFileRemoval(opts cmdutil.Options, productID string, removed []existingProductFile) (bool, error) {
	if len(removed) == 0 {
		return true, nil
	}
	return cmdutil.ConfirmAction(opts, productFileRemovalMessage(productID, removed))
}

func dryRunExistingProductFile(file existingProductFile) dryRunExistingFile {
	return dryRunExistingFile{
		ID:   file.ID,
		Name: file.Name,
	}
}

func formatExistingProductFileLabel(file existingProductFile) string {
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

func summarizeExistingProductFiles(files []existingProductFile, max int) string {
	if len(files) == 0 {
		return ""
	}
	if max <= 0 || max > len(files) {
		max = len(files)
	}
	labels := make([]string, 0, max+1)
	for _, current := range files[:max] {
		labels = append(labels, formatExistingProductFileLabel(current))
	}
	if extra := len(files) - max; extra > 0 {
		labels = append(labels, fmt.Sprintf("and %d more", extra))
	}
	return strings.Join(labels, ", ")
}

func productFileRemovalMessage(productID string, removed []existingProductFile) string {
	label := "1 existing file"
	if len(removed) != 1 {
		label = strconv.Itoa(len(removed)) + " existing files"
	}

	message := fmt.Sprintf("Update product %s and remove %s?", productID, label)
	if summary := summarizeExistingProductFiles(removed, 5); summary != "" {
		message = fmt.Sprintf("Update product %s and remove %s: %s?", productID, label, summary)
	}
	return message
}

func joinComma(values []string) string {
	return strings.Join(values, ", ")
}

func productBatchUploadInputs(uploads []plannedProductUpload) []batchUploadInput {
	inputs := make([]batchUploadInput, len(uploads))
	for i, current := range uploads {
		inputs[i] = batchUploadInput{
			Path: current.Path,
			Plan: current.Plan,
		}
	}
	return inputs
}
