package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

func hasTestItems(page testPage) bool {
	return len(page.Items) > 0
}

func walkTestPages(pages []testPage) func(PageVisitor[testPage]) error {
	return func(visit PageVisitor[testPage]) error {
		for _, page := range pages {
			stop, err := visit(page)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		return nil
	}
}

func writeTestItems(page testPage, writeItem func(any) error) error {
	for _, item := range page.Items {
		if err := writeItem(item); err != nil {
			return err
		}
	}
	return nil
}

func writeTestPlainPage(w io.Writer, page testPage) error {
	rows := make([][]string, 0, len(page.Items))
	for _, item := range page.Items {
		rows = append(rows, []string{item})
	}
	return output.PrintPlain(w, rows)
}

func writeTestTablePage(w io.Writer, page testPage) error {
	_, err := io.WriteString(w, strings.Join(page.Items, ",")+"\n")
	return err
}

func TestRequireNonNegativeDurationFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Duration("page-delay", 0, "")

	if err := cmd.Flags().Set("page-delay", "0s"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireNonNegativeDurationFlag(cmd, "page-delay", 0); err != nil {
		t.Fatalf("zero should be allowed, got %v", err)
	}

	if err := cmd.Flags().Set("page-delay", "-1s"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := RequireNonNegativeDurationFlag(cmd, "page-delay", -1*time.Second)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"--page-delay cannot be negative", "Usage:", "demo [flags]"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func TestStreamPaginatedPages_JSONOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.JSONOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		JSONKey: "items",
		Walk: walkTestPages([]testPage{
			{Items: []string{"one"}, NextPageKey: "page-2"},
			{Items: []string{"two"}},
		}),
		WriteItems: writeTestItems,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}

	var payload struct {
		Success bool     `json:"success"`
		Items   []string `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if !payload.Success || strings.Join(payload.Items, ",") != "one,two" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestStreamPaginatedPages_JQOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.JQExpr = ".items[]"
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		JSONKey: "items",
		Walk: walkTestPages([]testPage{
			{Items: []string{"one"}},
			{Items: []string{"two"}},
		}),
		WriteItems: writeTestItems,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "\"one\"\n\"two\"" {
		t.Fatalf("got %q, want %q", got, "\"one\"\n\"two\"")
	}
}

func TestStreamPaginatedPages_PlainOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		EmptyMessage: "No items found.",
		Walk: walkTestPages([]testPage{
			{},
			{Items: []string{"one", "two"}},
		}),
		HasItems:       hasTestItems,
		WritePlainPage: writeTestPlainPage,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "one\ntwo" {
		t.Fatalf("got %q, want %q", got, "one\ntwo")
	}
}

func TestStreamPaginatedPages_PlainOutputEmptyState(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		EmptyMessage:   "No items found.",
		Walk:           walkTestPages([]testPage{{}, {}}),
		HasItems:       hasTestItems,
		WritePlainPage: writeTestPlainPage,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "No items found." {
		t.Fatalf("got %q, want %q", got, "No items found.")
	}
}

func TestStreamPaginatedPages_TableOutputSeparatesPages(t *testing.T) {
	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		EmptyMessage: "No items found.",
		Walk: walkTestPages([]testPage{
			{Items: []string{"one"}},
			{},
			{Items: []string{"two"}},
		}),
		HasItems:       hasTestItems,
		WriteTablePage: writeTestTablePage,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}
	if got := out.String(); got != "one\n\ntwo\n" {
		t.Fatalf("got %q, want %q", got, "one\\n\\ntwo\\n")
	}
}

func TestStreamPaginatedPages_TableOutputEmptyState(t *testing.T) {
	opts := DefaultOptions()
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		EmptyMessage:   "No items found.",
		Walk:           walkTestPages([]testPage{{}, {}}),
		HasItems:       hasTestItems,
		WriteTablePage: writeTestTablePage,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "No items found." {
		t.Fatalf("got %q, want %q", got, "No items found.")
	}
}

func TestStreamPaginatedPages_EmptyStateHonorsQuiet(t *testing.T) {
	opts := DefaultOptions()
	opts.Quiet = true
	var out bytes.Buffer
	opts.Stdout = &out

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		EmptyMessage:   "No items found.",
		Walk:           walkTestPages([]testPage{{}, {}}),
		HasItems:       hasTestItems,
		WriteTablePage: writeTestTablePage,
	})
	if err != nil {
		t.Fatalf("StreamPaginatedPages failed: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no quiet output, got %q", out.String())
	}
}

func TestStreamPaginatedPages_JSONRequiresItemWriter(t *testing.T) {
	opts := DefaultOptions()
	opts.JSONOutput = true

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		Walk: walkTestPages([]testPage{{Items: []string{"one"}}}),
	})
	if err == nil || !strings.Contains(err.Error(), "paginated item writer is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamPaginatedPages_JQRequiresItemWriter(t *testing.T) {
	opts := DefaultOptions()
	opts.JQExpr = ".items[]"

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		Walk: walkTestPages([]testPage{{Items: []string{"one"}}}),
	})
	if err == nil || !strings.Contains(err.Error(), "paginated item writer is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamPaginatedPages_PlainRequiresWriters(t *testing.T) {
	opts := DefaultOptions()
	opts.PlainOutput = true

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		Walk: walkTestPages([]testPage{{Items: []string{"one"}}}),
	})
	if err == nil || !strings.Contains(err.Error(), "paginated item detector is required") {
		t.Fatalf("unexpected detector error: %v", err)
	}

	err = StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		Walk:     walkTestPages([]testPage{{Items: []string{"one"}}}),
		HasItems: hasTestItems,
	})
	if err == nil || !strings.Contains(err.Error(), "paginated plain writer is required") {
		t.Fatalf("unexpected writer error: %v", err)
	}
}

func TestStreamPaginatedPages_TableRequiresWriters(t *testing.T) {
	err := StreamPaginatedPages(DefaultOptions(), PaginatedPageOutputConfig[testPage]{
		Walk: walkTestPages([]testPage{{Items: []string{"one"}}}),
	})
	if err == nil || !strings.Contains(err.Error(), "paginated item detector is required") {
		t.Fatalf("unexpected detector error: %v", err)
	}

	err = StreamPaginatedPages(DefaultOptions(), PaginatedPageOutputConfig[testPage]{
		Walk:     walkTestPages([]testPage{{Items: []string{"one"}}}),
		HasItems: hasTestItems,
	})
	if err == nil || !strings.Contains(err.Error(), "paginated table writer is required") {
		t.Fatalf("unexpected writer error: %v", err)
	}
}

func TestStreamPaginatedPages_RequiresWalk(t *testing.T) {
	err := StreamPaginatedPages(DefaultOptions(), PaginatedPageOutputConfig[testPage]{})
	if err == nil || !strings.Contains(err.Error(), "paginated walk is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamPaginatedPages_PropagatesWriterError(t *testing.T) {
	want := errors.New("boom")
	opts := DefaultOptions()
	opts.PlainOutput = true

	err := StreamPaginatedPages(opts, PaginatedPageOutputConfig[testPage]{
		Walk:     walkTestPages([]testPage{{Items: []string{"one"}}}),
		HasItems: hasTestItems,
		WritePlainPage: func(io.Writer, testPage) error {
			return want
		},
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}
