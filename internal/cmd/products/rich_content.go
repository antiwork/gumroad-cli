package products

import (
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
	existingFiles []existingProductFile,
	fileRefs []richContentFileRef,
) ([]map[string]any, bool, error) {
	if len(fileRefs) == 0 {
		return nil, false, nil
	}

	richContent, err := rollFileEmbeds(existingRichContent, existingFiles, fileRefs)
	if err != nil {
		return nil, false, cmdutil.UsageErrorf(cmd, "%s; pass one --file per existing file embed, or use products content get/set for structural content changes", err.Error())
	}
	return richContent, true, nil
}

func rollFileEmbeds(richContent []map[string]any, preserved []existingProductFile, fileRefs []richContentFileRef) ([]map[string]any, error) {
	return richcontent.RollFileEmbeds(richContent, preservedProductFileIDs(preserved), fileRefs)
}

func preservedProductFileIDs(files []existingProductFile) []string {
	ids := make([]string, len(files))
	for i, file := range files {
		ids[i] = file.ID
	}
	return ids
}

func fileEmbedIDs(richContent []map[string]any) []string {
	return richcontent.FileEmbedIDs(richContent)
}
