package products

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/richcontent"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/spf13/cobra"
)

// Course upload writes the product once per page instead of once per file. The resume
// file records each finished upload and page so an interrupted run can be repeated
// without uploading a file twice or placing an embed twice.

const defaultCourseStateSuffix = ".state.json"

type courseFileSpec struct {
	Path        string
	DisplayName string
	Description string
}

type coursePageSpec struct {
	ID    string
	Title string
	Files []courseFileSpec
}

type courseManifest struct {
	Pages []coursePageSpec
}

type courseUploadState struct {
	Version   int               `json:"version"`
	ProductID string            `json:"product_id,omitempty"`
	Uploads   map[string]string `json:"uploads,omitempty"`
	Pages     map[string]bool   `json:"pages,omitempty"`
}

type courseUploadSummary struct {
	ProductID    string `json:"product_id"`
	Pages        int    `json:"pages"`
	PagesWritten int    `json:"pages_written"`
	PagesSkipped int    `json:"pages_skipped"`
	Files        int    `json:"files"`
	Uploads      int    `json:"uploads"`
	StatePath    string `json:"state_path,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
}

func newContentUploadCmd() *cobra.Command {
	var statePath string

	cmd := &cobra.Command{
		Use:   "upload <product_id> [course.json]",
		Short: "Upload a whole course with one product write per page",
		Long: "Upload every file in a course and place it on its page, with one product write per page.\n\n" +
			"The manifest maps page ids (as listed by `products content list`) to local files:\n\n" +
			"  {\"pages\": [{\"id\": \"week-1\", \"title\": \"Week 1\", \"files\": [\"./intro.mp4\", {\"path\": \"./notes.pdf\", \"name\": \"Notes\"}]}]}\n\n" +
			"A file entry is a path or an object with path, name and description. Every path is checked before the first upload starts. " +
			"Finished uploads and pages are recorded in a resume file, so re-running after an interruption neither uploads a file twice nor places an embed twice.",
		Example: `  gumroad products content upload <product_id> course.json
  gumroad products content upload <product_id> course.json --state ./my-course.state.json
  gumroad products content upload <product_id> course.json --dry-run`,
		Args: productContentSetArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			productID := args[0]

			manifestPath := productContentPathWithDefault(args, "./course.json")
			manifest, err := readCourseManifest(manifestPath)
			if err != nil {
				return cmdutil.InvalidInputErrorf("%s", err)
			}
			if err := validateCourseManifest(manifest); err != nil {
				return cmdutil.InvalidInputErrorf("%s", err)
			}

			if statePath == "" {
				statePath = manifestPath + defaultCourseStateSuffix
			}
			state, err := readCourseUploadState(statePath)
			if err != nil {
				return cmdutil.InvalidInputErrorf("%s", err)
			}
			if state.ProductID != "" && state.ProductID != productID {
				return cmdutil.InvalidInputErrorf("resume file %s belongs to product %s, not %s; pass --state for a separate file", statePath, state.ProductID, productID)
			}
			state.ProductID = productID

			if opts.DryRun {
				return printCourseUploadPlan(opts, productID, manifest, 0, 0, statePath)
			}

			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)
			summary, err := runCourseUpload(opts, client, productID, manifest, state, statePath)
			if err != nil {
				return err
			}
			return printCourseUploadSummary(opts, summary)
		},
	}

	cmd.Flags().StringVar(&statePath, "state", "", "Resume state file (defaults to <course.json>"+defaultCourseStateSuffix+")")
	return cmd
}

func readCourseManifest(path string) (courseManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return courseManifest{}, fmt.Errorf("cannot read course manifest %s: %w", path, err)
	}
	return parseCourseManifest(data)
}

func parseCourseManifest(data []byte) (courseManifest, error) {
	var raw struct {
		Pages []json.RawMessage `json:"pages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return courseManifest{}, fmt.Errorf("course manifest is not valid JSON: %w", err)
	}
	if len(raw.Pages) == 0 {
		return courseManifest{}, fmt.Errorf("course manifest needs at least one page under \"pages\"")
	}

	manifest := courseManifest{Pages: make([]coursePageSpec, len(raw.Pages))}
	for i, entry := range raw.Pages {
		var page struct {
			ID    string            `json:"id"`
			Page  string            `json:"page"`
			Title string            `json:"title"`
			Files []json.RawMessage `json:"files"`
		}
		if err := json.Unmarshal(entry, &page); err != nil {
			return courseManifest{}, fmt.Errorf("page %d is not a valid page object: %w", i+1, err)
		}
		pageID := strings.TrimSpace(page.ID)
		if pageID == "" {
			pageID = strings.TrimSpace(page.Page)
		}
		if pageID == "" {
			return courseManifest{}, fmt.Errorf("page %d has no id; take it from `gumroad products content list <product_id>`", i+1)
		}
		files := make([]courseFileSpec, len(page.Files))
		for j, fileEntry := range page.Files {
			spec, err := parseCourseFileSpec(fileEntry)
			if err != nil {
				return courseManifest{}, fmt.Errorf("page %s file %d: %w", pageID, j+1, err)
			}
			files[j] = spec
		}
		manifest.Pages[i] = coursePageSpec{ID: pageID, Title: strings.TrimSpace(page.Title), Files: files}
	}
	return manifest, nil
}

func parseCourseFileSpec(data json.RawMessage) (courseFileSpec, error) {
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		if strings.TrimSpace(path) == "" {
			return courseFileSpec{}, fmt.Errorf("file path is empty")
		}
		return courseFileSpec{Path: path}, nil
	}

	var object struct {
		Path        string `json:"path"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return courseFileSpec{}, fmt.Errorf("file entry must be a path or an object with \"path\": %w", err)
	}
	if strings.TrimSpace(object.Path) == "" {
		return courseFileSpec{}, fmt.Errorf("file entry is missing \"path\"")
	}
	return courseFileSpec{
		Path:        object.Path,
		DisplayName: strings.TrimSpace(object.Name),
		Description: strings.TrimSpace(object.Description),
	}, nil
}

// validateCourseManifest runs before the first upload so an unusable manifest fails
// while nothing has been uploaded yet.
func validateCourseManifest(manifest courseManifest) error {
	seenPages := map[string]bool{}
	for _, page := range manifest.Pages {
		if seenPages[page.ID] {
			return fmt.Errorf("page %s is listed twice", page.ID)
		}
		seenPages[page.ID] = true
		for _, file := range page.Files {
			info, err := os.Stat(file.Path)
			if err != nil {
				return fmt.Errorf("page %s: cannot read %s: %w", page.ID, file.Path, err)
			}
			if info.IsDir() {
				return fmt.Errorf("page %s: %s is a directory, not a file", page.ID, file.Path)
			}
		}
	}
	return nil
}

func readCourseUploadState(path string) (courseUploadState, error) {
	state := courseUploadState{
		Version: 1,
		Uploads: map[string]string{},
		Pages:   map[string]bool{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("cannot read resume file %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("resume file %s is not valid JSON: %w", path, err)
	}
	if state.Uploads == nil {
		state.Uploads = map[string]string{}
	}
	if state.Pages == nil {
		state.Pages = map[string]bool{}
	}
	return state, nil
}

func writeCourseUploadState(path string, state courseUploadState) error {
	state.Version = 1
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode resume file: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("cannot write resume file %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("cannot write resume file %s: %w", path, err)
	}
	return nil
}

func runCourseUpload(
	opts cmdutil.Options,
	client *api.Client,
	productID string,
	manifest courseManifest,
	state courseUploadState,
	statePath string,
) (courseUploadSummary, error) {
	summary := courseUploadSummary{ProductID: productID, Pages: len(manifest.Pages), StatePath: statePath}
	for _, page := range manifest.Pages {
		summary.Files += len(page.Files)
	}

	productState, err := fetchExistingProductFileState(client, productID)
	if err != nil {
		return summary, err
	}
	if productUsesPerVariantRichContent(productState) {
		return summary, cmdutil.InvalidInputErrorf("product %s uses per-variant rich content; course upload writes shared content only", productID)
	}
	richContent, err := richcontent.Clone(productState.RichContent)
	if err != nil {
		return summary, err
	}
	if richContent == nil {
		richContent = []map[string]any{}
	}

	for _, page := range manifest.Pages {
		if state.Pages[page.ID] {
			summary.PagesSkipped++
			continue
		}

		urls := make([]string, len(page.Files))
		for i, file := range page.Files {
			if resumeURL := state.Uploads[file.Path]; resumeURL != "" {
				urls[i] = resumeURL
				continue
			}
			plan, err := upload.Describe(file.Path, upload.Options{Filename: file.DisplayName})
			if err != nil {
				return summary, err
			}
			fileURL, err := uploadBatch(opts, client, []batchUploadInput{{Path: file.Path, Plan: plan}})
			if err != nil {
				return summary, err
			}
			urls[i] = fileURL[0]
			summary.Uploads++
			state.Uploads[file.Path] = fileURL[0]
			if err := writeCourseUploadState(statePath, state); err != nil {
				return summary, err
			}
		}

		refs, err := richcontent.NewFileRefs(len(page.Files))
		if err != nil {
			return summary, err
		}
		pageDocument, err := json.Marshal(richcontent.BuildFilePage(page.ID, page.Title, refs))
		if err != nil {
			return summary, fmt.Errorf("could not encode page %s: %w", page.ID, err)
		}
		existingDocument, err := json.Marshal(richContent)
		if err != nil {
			return summary, fmt.Errorf("could not encode rich content: %w", err)
		}
		mergedDocument, err := upsertRichContentPage(existingDocument, pageDocument, page.ID)
		if err != nil {
			return summary, err
		}
		var nextRichContent []map[string]any
		if err := json.Unmarshal(mergedDocument, &nextRichContent); err != nil {
			return summary, fmt.Errorf("could not decode merged rich content: %w", err)
		}

		plan := productFileUpdatePlan{Preserved: productState.Files, Uploads: courseRequestedUploads(page.Files)}
		body := map[string]any{
			"files":        buildProductUpdateFilesPayload(plan, urls, refs),
			"rich_content": nextRichContent,
		}
		path := cmdutil.JoinPath("products", productID)
		if _, err := runContentSetJSONData(opts, client, path, body); err != nil {
			return summary, fmt.Errorf("page %s: %w", page.ID, err)
		}
		summary.PagesWritten++
		richContent = nextRichContent
		state.Pages[page.ID] = true
		if err := writeCourseUploadState(statePath, state); err != nil {
			return summary, err
		}

		// The server assigns real file ids on each write; re-read them so the next page
		// preserves every file placed so far.
		productState, err = fetchExistingProductFileState(client, productID)
		if err != nil {
			return summary, err
		}
	}

	return summary, nil
}

func courseRequestedUploads(files []courseFileSpec) []requestedProductUpload {
	uploads := make([]requestedProductUpload, len(files))
	for i, file := range files {
		uploads[i] = requestedProductUpload{
			Path:        file.Path,
			DisplayName: file.DisplayName,
			Description: file.Description,
		}
	}
	return uploads
}

func printCourseUploadPlan(opts cmdutil.Options, productID string, manifest courseManifest, pagesSkipped, filesSkipped int, statePath string) error {
	summary := courseUploadSummary{
		ProductID:    productID,
		Pages:        len(manifest.Pages),
		PagesSkipped: pagesSkipped,
		StatePath:    statePath,
		DryRun:       true,
	}
	for _, page := range manifest.Pages {
		summary.Files += len(page.Files)
	}

	if opts.UsesJSONOutput() {
		data, err := json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+fmt.Sprintf(": %d product writes for %d files across %d pages", len(manifest.Pages)-pagesSkipped, summary.Files, len(manifest.Pages))); err != nil {
		return err
	}
	return output.Writeln(opts.Out(), "No files uploaded.")
}

func printCourseUploadSummary(opts cmdutil.Options, summary courseUploadSummary) error {
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("could not encode output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}

	message := fmt.Sprintf("Uploaded %d files and wrote %d pages (%d product writes)", summary.Uploads, summary.PagesWritten, summary.PagesWritten)
	if summary.PagesSkipped > 0 {
		message += fmt.Sprintf("; skipped %d already-written pages", summary.PagesSkipped)
	}
	return output.Writeln(opts.Out(), message)
}
