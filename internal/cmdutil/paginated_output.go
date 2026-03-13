package cmdutil

import (
	"fmt"
	"io"

	"github.com/antiwork/gumroad-cli/internal/output"
)

// PaginatedPageOutputConfig describes how a paginated command should render
// pages across machine-readable and human output modes.
type PaginatedPageOutputConfig[T any] struct {
	JSONKey        string
	EmptyMessage   string
	Walk           func(PageVisitor[T]) error
	HasItems       func(T) bool
	WriteItems     func(T, func(any) error) error
	WritePlainPage func(io.Writer, T) error
	WriteTablePage func(io.Writer, T) error
}

type paginatedItemWriter func(func(any) error) error
type paginatedItemPrinter func(paginatedItemWriter) error

// StreamPaginatedPages coordinates output-mode specific rendering for `--all`
// style paginated commands while preserving atomic JSON/JQ output.
func StreamPaginatedPages[T any](opts Options, cfg PaginatedPageOutputConfig[T]) error {
	switch {
	case opts.JQExpr != "":
		return streamPaginatedJSONWithJQ(opts, cfg)
	case opts.JSONOutput:
		return streamPaginatedJSON(opts, cfg)
	case opts.PlainOutput:
		return streamPaginatedPlain(opts, cfg)
	default:
		return streamPaginatedTable(opts, cfg)
	}
}

func streamPaginatedJSON[T any](opts Options, cfg PaginatedPageOutputConfig[T]) error {
	return streamPaginatedItems(opts, cfg, func(writeItems paginatedItemWriter) error {
		return output.PrintJSONStream(opts.Out(), cfg.JSONKey, writeItems)
	})
}

func streamPaginatedJSONWithJQ[T any](opts Options, cfg PaginatedPageOutputConfig[T]) error {
	return streamPaginatedItems(opts, cfg, func(writeItems paginatedItemWriter) error {
		return output.PrintJSONStreamWithJQ(opts.Out(), cfg.JSONKey, opts.JQExpr, writeItems)
	})
}

func streamPaginatedItems[T any](opts Options, cfg PaginatedPageOutputConfig[T], print paginatedItemPrinter) error {
	if err := requirePaginatedWalk(cfg.Walk); err != nil {
		return err
	}
	if cfg.WriteItems == nil {
		return fmt.Errorf("paginated item writer is required")
	}

	return print(func(writeItem func(any) error) error {
		return cfg.Walk(func(page T) (bool, error) {
			return false, cfg.WriteItems(page, writeItem)
		})
	})
}

func streamPaginatedPlain[T any](opts Options, cfg PaginatedPageOutputConfig[T]) error {
	if err := requirePaginatedWalk(cfg.Walk); err != nil {
		return err
	}
	if cfg.HasItems == nil {
		return fmt.Errorf("paginated item detector is required")
	}
	if cfg.WritePlainPage == nil {
		return fmt.Errorf("paginated plain writer is required")
	}

	foundAny := false
	err := cfg.Walk(func(page T) (bool, error) {
		if !cfg.HasItems(page) {
			return false, nil
		}
		foundAny = true
		return false, cfg.WritePlainPage(opts.Out(), page)
	})
	if err != nil {
		return err
	}
	if !foundAny {
		return PrintInfo(opts, cfg.EmptyMessage)
	}
	return nil
}

func streamPaginatedTable[T any](opts Options, cfg PaginatedPageOutputConfig[T]) error {
	if err := requirePaginatedWalk(cfg.Walk); err != nil {
		return err
	}
	if cfg.HasItems == nil {
		return fmt.Errorf("paginated item detector is required")
	}
	if cfg.WriteTablePage == nil {
		return fmt.Errorf("paginated table writer is required")
	}

	foundAny := false
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		wrotePage := false
		err := cfg.Walk(func(page T) (bool, error) {
			if !cfg.HasItems(page) {
				return false, nil
			}
			foundAny = true
			if wrotePage {
				if err := output.Writeln(w); err != nil {
					return false, err
				}
			}
			if err := cfg.WriteTablePage(w, page); err != nil {
				return false, err
			}
			wrotePage = true
			return false, nil
		})
		if err != nil {
			return err
		}
		if !foundAny && !opts.Quiet {
			return output.Writeln(w, cfg.EmptyMessage)
		}
		return nil
	})
}

func requirePaginatedWalk[T any](walk func(PageVisitor[T]) error) error {
	if walk == nil {
		return fmt.Errorf("paginated walk is required")
	}
	return nil
}
