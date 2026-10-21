package products

import (
	"github.com/antiwork/gumroad-cli/internal/richcontent"
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
	existingRichContent []map[string]any,
	existingFiles []existingProductFile,
	pageID string,
	fileRefs []richContentFileRef,
) ([]map[string]any, bool, error) {
	if len(fileRefs) == 0 {
		return nil, false, nil
	}

	var (
		richContent []map[string]any
		err         error
	)
	if pageID != "" {
		richContent, err = richcontent.AppendFileEmbedsToPage(existingRichContent, pageID, fileRefs)
	} else {
		richContent, err = richcontent.AppendFileEmbeds(existingRichContent, preservedProductFileIDs(existingFiles), fileRefs)
	}
	if err != nil {
		return nil, false, err
	}
	return richContent, true, nil
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
