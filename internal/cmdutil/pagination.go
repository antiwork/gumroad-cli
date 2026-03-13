package cmdutil

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
)

type PageVisitor[T any] func(T) (stop bool, err error)

// FetchPage fetches and decodes a single Gumroad page response.
func FetchPage[T any](client *api.Client, path string, params url.Values) (T, error) {
	var zero T
	if client == nil {
		return zero, fmt.Errorf("nil api client")
	}

	data, err := client.Get(path, params)
	if err != nil {
		return zero, err
	}
	return DecodeJSON[T](data)
}

// WalkPages follows Gumroad-style next_page_key pagination and decodes each
// response into T before visiting it.
func WalkPages[T any](client *api.Client, path string, params url.Values, nextPageKey func(T) string, visit PageVisitor[T]) error {
	return WalkPagesWithDelay[T](context.Background(), 0, client, path, params, nextPageKey, visit)
}

// WalkPagesWithDelay follows Gumroad-style next_page_key pagination, pausing
// between follow-up page requests when delay > 0.
func WalkPagesWithDelay[T any](ctx context.Context, delay time.Duration, client *api.Client, path string, params url.Values, nextPageKey func(T) string, visit PageVisitor[T]) error {
	firstPage, err := FetchPage[T](client, path, params)
	if err != nil {
		return err
	}
	return walkDecodedPagesWithDelay(ctx, delay, params, firstPage, nextPageKey, func(query url.Values) (T, error) {
		return FetchPage[T](client, path, query)
	}, visit)
}

// walkDecodedPages keeps the page transition logic in one place so paginated
// commands share request ordering, page_key mutation, and cycle detection.
func walkDecodedPages[T any](params url.Values, firstPage T, nextPageKey func(T) string, fetch func(url.Values) (T, error), visit PageVisitor[T]) error {
	return walkDecodedPagesWithDelay(context.Background(), 0, params, firstPage, nextPageKey, fetch, visit)
}

var sleepPageDelay = func(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func walkDecodedPagesWithDelay[T any](ctx context.Context, delay time.Duration, params url.Values, firstPage T, nextPageKey func(T) string, fetch func(url.Values) (T, error), visit PageVisitor[T]) error {
	page := firstPage
	query := CloneValues(params)
	tracker := newPageKeyTracker(query)

	for {
		stop, err := visit(page)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}

		pageKey := nextPageKey(page)
		if pageKey == "" {
			return nil
		}

		if err := tracker.Track(pageKey); err != nil {
			return err
		}
		if err := sleepPageDelay(ctx, delay); err != nil {
			return err
		}
		query.Set("page_key", pageKey)
		nextPage, err := fetch(query)
		if err != nil {
			return err
		}
		page = nextPage
	}
}

type pageKeyTracker struct {
	seen map[string]struct{}
}

func newPageKeyTracker(params url.Values) *pageKeyTracker {
	tracker := &pageKeyTracker{seen: make(map[string]struct{})}
	if pageKey := params.Get("page_key"); pageKey != "" {
		tracker.seen[pageKey] = struct{}{}
	}
	return tracker
}

// Track rejects repeated page cursors so a buggy or cyclical API response
// cannot trap `--all` commands in an infinite loop.
func (t *pageKeyTracker) Track(pageKey string) error {
	if pageKey == "" {
		return nil
	}
	if _, ok := t.seen[pageKey]; ok {
		return fmt.Errorf("pagination cycle detected for page_key %q", pageKey)
	}
	t.seen[pageKey] = struct{}{}
	return nil
}
