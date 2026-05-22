package products

import (
	"sort"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/richcontent"
	"github.com/spf13/cobra"
)

const defaultFileRichContentTitle = richcontent.DefaultFileTitle

type richContentFileRef = richcontent.FileRef

func newRichContentFileRefs(count int) ([]richContentFileRef, error) {
	return richcontent.NewFileRefs(count)
}

func buildFileRichContent(fileRefs []richContentFileRef) []map[string]any {
	return richcontent.BuildFileRichContent(fileRefs)
}

func buildProductUpdateRichContent(
	cmd *cobra.Command,
	existingRichContent []map[string]any,
	filePlan productFileUpdatePlan,
	fileRefs []richContentFileRef,
) ([]map[string]any, bool, error) {
	removedEmbeddedIDs := removedFileEmbedIDs(existingRichContent, filePlan.Removed)
	if len(removedEmbeddedIDs) > 0 {
		if len(fileRefs) == 0 {
			richContent, err := cloneRichContent(existingRichContent)
			if err != nil {
				return nil, false, err
			}
			stripFileEmbedIDs(richContent, removedEmbeddedIDs)
			return richContent, true, nil
		}
		if len(removedEmbeddedIDs) != 1 || len(fileRefs) != 1 {
			return nil, false, cmdutil.UsageErrorf(cmd,
				"cannot automatically update rich_content for embedded file removal (%s); replace one embedded file at a time with exactly one --remove-file and one --file",
				joinComma(removedEmbeddedIDs))
		}

		richContent, err := cloneRichContent(existingRichContent)
		if err != nil {
			return nil, false, err
		}
		replaceFileEmbedID(richContent, removedEmbeddedIDs[0], fileRefs[0].FileID)
		return richContent, true, nil
	}

	if len(fileRefs) == 0 {
		return nil, false, nil
	}

	richContent, err := appendFileEmbeds(existingRichContent, filePlan.Preserved, fileRefs)
	if err != nil {
		return nil, false, err
	}
	return richContent, true, nil
}

func removedFileEmbedIDs(richContent []map[string]any, removed []existingProductFile) []string {
	if len(richContent) == 0 || len(removed) == 0 {
		return nil
	}

	removedIDs := make(map[string]struct{}, len(removed))
	for _, file := range removed {
		removedIDs[file.ID] = struct{}{}
	}

	seen := map[string]struct{}{}
	for _, id := range fileEmbedIDs(richContent) {
		if _, ok := removedIDs[id]; !ok {
			continue
		}
		seen[id] = struct{}{}
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func appendFileEmbeds(richContent []map[string]any, preserved []existingProductFile, fileRefs []richContentFileRef) ([]map[string]any, error) {
	return richcontent.AppendFileEmbeds(richContent, preservedProductFileIDs(preserved), fileRefs)
}

func preservedProductFileIDs(files []existingProductFile) []string {
	ids := make([]string, len(files))
	for i, file := range files {
		ids[i] = file.ID
	}
	return ids
}

func cloneRichContent(richContent []map[string]any) ([]map[string]any, error) {
	return richcontent.Clone(richContent)
}

func fileEmbedIDs(richContent []map[string]any) []string {
	return richcontent.FileEmbedIDs(richContent)
}

func replaceFileEmbedID(richContent []map[string]any, oldID, newID string) {
	richcontent.ReplaceFileEmbedID(richContent, oldID, newID)
}

func stripFileEmbedIDs(richContent []map[string]any, ids []string) {
	richcontent.StripFileEmbedIDs(richContent, ids)
}
