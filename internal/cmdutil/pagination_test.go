package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/spf13/cobra"
)

type testPage struct {
	Items       []string `json:"items"`
	NextPageKey string   `json:"next_page_key"`
}

func TestRequirePositiveIntFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Int("amount-cents", 0, "")

	if err := RequirePositiveIntFlag(cmd, "amount-cents", 0); err != nil {
		t.Fatalf("unchanged flag should be ignored, got %v", err)
	}

	if err := cmd.Flags().Set("amount-cents", "-5"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := RequirePositiveIntFlag(cmd, "amount-cents", -5)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"--amount-cents must be greater than 0", "Usage:", "demo [flags]"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func TestRequireNonNegativeIntFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Int("max-purchase-count", 0, "")

	if err := cmd.Flags().Set("max-purchase-count", "0"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := RequireNonNegativeIntFlag(cmd, "max-purchase-count", 0); err != nil {
		t.Fatalf("zero should be allowed, got %v", err)
	}

	if err := cmd.Flags().Set("max-purchase-count", "-1"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := RequireNonNegativeIntFlag(cmd, "max-purchase-count", -1)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"--max-purchase-count cannot be negative", "Usage:", "demo [flags]"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func TestWalkPagesFollowsNextPageKey(t *testing.T) {
	var gotQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueries = append(gotQueries, r.URL.RawQuery)

		payload := map[string]any{"items": []string{r.URL.Query().Get("page_key")}}
		switch r.URL.Query().Get("page_key") {
		case "":
			payload["next_page_key"] = "page-2"
		case "page-2":
			payload["next_page_key"] = ""
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}

		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	client := api.NewClient("test-token", "test", false)

	var seen []string
	err := WalkPages[testPage](client, "/items", url.Values{"filter": {"recent"}}, func(page testPage) string {
		return page.NextPageKey
	}, func(page testPage) (bool, error) {
		seen = append(seen, page.Items...)
		return false, nil
	})
	if err != nil {
		t.Fatalf("WalkPages failed: %v", err)
	}
	if !reflect.DeepEqual(gotQueries, []string{"filter=recent", "filter=recent&page_key=page-2"}) {
		t.Fatalf("got queries %v", gotQueries)
	}
	if !reflect.DeepEqual(seen, []string{"", "page-2"}) {
		t.Fatalf("got seen items %v", seen)
	}
}

func TestWalkPagesStopsEarly(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := json.NewEncoder(w).Encode(map[string]any{"next_page_key": "page-2"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	client := api.NewClient("test-token", "test", false)

	err := WalkPages[testPage](client, "/items", nil, func(page testPage) string {
		return page.NextPageKey
	}, func(testPage) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("WalkPages failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
}

func TestWalkPagesWithDelay_SleepsBetweenPages(t *testing.T) {
	origSleep := sleepPageDelay
	t.Cleanup(func() { sleepPageDelay = origSleep })

	var slept []time.Duration
	sleepPageDelay = func(ctx context.Context, delay time.Duration) error {
		slept = append(slept, delay)
		return nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"items":         []string{"page-1"},
				"next_page_key": "page-2",
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "page-2":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"items": []string{"page-2"},
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	}))
	defer srv.Close()

	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	client := api.NewClient("test-token", "test", false)

	err := WalkPagesWithDelay[testPage](context.Background(), 250*time.Millisecond, client, "/items", nil, func(page testPage) string {
		return page.NextPageKey
	}, func(testPage) (bool, error) {
		return false, nil
	})
	if err != nil {
		t.Fatalf("WalkPagesWithDelay failed: %v", err)
	}
	if !reflect.DeepEqual(slept, []time.Duration{250 * time.Millisecond}) {
		t.Fatalf("got slept=%v, want [250ms]", slept)
	}
}

func TestWalkPagesWithDelay_StopsOnCancelledContext(t *testing.T) {
	origSleep := sleepPageDelay
	t.Cleanup(func() { sleepPageDelay = origSleep })

	sleepPageDelay = func(ctx context.Context, delay time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := json.NewEncoder(w).Encode(map[string]any{
			"items":         []string{"page-1"},
			"next_page_key": "page-2",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	client := api.NewClient("test-token", "test", false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := WalkPagesWithDelay[testPage](ctx, 250*time.Millisecond, client, "/items", nil, func(page testPage) string {
		return page.NextPageKey
	}, func(testPage) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context canceled", err)
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
}

func TestWalkPages_PropagatesSecondPageAPIError(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"items":         []string{"page-1"},
				"next_page_key": "page-2",
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "page-2":
			w.WriteHeader(http.StatusForbidden)
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": "upstream failure",
			}); err != nil {
				t.Fatalf("encode error response: %v", err)
			}
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	}))
	defer srv.Close()

	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	client := api.NewClient("test-token", "test", false)

	var seen []string
	err := WalkPages[testPage](client, "/items", nil, func(page testPage) string {
		return page.NextPageKey
	}, func(page testPage) (bool, error) {
		seen = append(seen, page.Items...)
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), "upstream failure") {
		t.Fatalf("expected second-page API error, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
	if !reflect.DeepEqual(seen, []string{"page-1"}) {
		t.Fatalf("got seen items %v", seen)
	}
}

func TestWalkPages_DetectsRepeatedPageKey(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"items":         []string{"page-1"},
				"next_page_key": "page-2",
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "page-2":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"items":         []string{"page-2"},
				"next_page_key": "page-2",
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	}))
	defer srv.Close()

	t.Setenv("GUMROAD_API_BASE_URL", srv.URL)
	client := api.NewClient("test-token", "test", false)

	err := WalkPages[testPage](client, "/items", nil, func(page testPage) string {
		return page.NextPageKey
	}, func(testPage) (bool, error) {
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), `pagination cycle detected for page_key "page-2"`) {
		t.Fatalf("expected pagination cycle error, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestWalkDecodedPagesFollowsNextPageKey(t *testing.T) {
	type page struct {
		Items       []string
		NextPageKey string
	}

	var gotQueries []string
	fetch := func(query url.Values) (page, error) {
		gotQueries = append(gotQueries, query.Encode())
		switch query.Get("page_key") {
		case "page-1":
			return page{Items: []string{"page-1"}, NextPageKey: "page-2"}, nil
		case "page-2":
			return page{Items: []string{"page-2"}}, nil
		default:
			t.Fatalf("unexpected page key %q", query.Get("page_key"))
			return page{}, nil
		}
	}

	var seen []string
	err := walkDecodedPages(url.Values{"filter": {"recent"}}, page{Items: []string{"page-0"}, NextPageKey: "page-1"}, func(current page) string {
		return current.NextPageKey
	}, fetch, func(current page) (bool, error) {
		seen = append(seen, current.Items...)
		return false, nil
	})
	if err != nil {
		t.Fatalf("WalkDecodedPages failed: %v", err)
	}
	if !reflect.DeepEqual(gotQueries, []string{"filter=recent&page_key=page-1", "filter=recent&page_key=page-2"}) {
		t.Fatalf("got queries %v", gotQueries)
	}
	if !reflect.DeepEqual(seen, []string{"page-0", "page-1", "page-2"}) {
		t.Fatalf("got seen items %v", seen)
	}
}

func TestWalkDecodedPages_PropagatesFetchError(t *testing.T) {
	type page struct {
		NextPageKey string
	}

	want := errors.New("boom")

	err := walkDecodedPages(nil, page{NextPageKey: "page-2"}, func(current page) string {
		return current.NextPageKey
	}, func(url.Values) (page, error) {
		return page{}, want
	}, func(page) (bool, error) {
		return false, nil
	})
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestWalkDecodedPages_DetectsRepeatedPageKey(t *testing.T) {
	type page struct {
		NextPageKey string
	}

	requests := 0
	err := walkDecodedPages(nil, page{NextPageKey: "page-1"}, func(current page) string {
		return current.NextPageKey
	}, func(query url.Values) (page, error) {
		requests++
		switch query.Get("page_key") {
		case "page-1":
			return page{NextPageKey: "page-1"}, nil
		default:
			t.Fatalf("unexpected page key %q", query.Get("page_key"))
			return page{}, nil
		}
	}, func(page) (bool, error) {
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), `pagination cycle detected for page_key "page-1"`) {
		t.Fatalf("expected pagination cycle error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
}
