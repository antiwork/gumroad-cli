package publicapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
)

func TestClientOmitsAuthorizationHeader(t *testing.T) {
	authHeaderSeen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, authHeaderSeen = r.Header["Authorization"]
		if err := json.NewEncoder(w).Encode(map[string]any{"products": []any{}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "test", false, srv.URL)
	if _, err := client.Get("/products/search.json", url.Values{"query": {"design"}}); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if authHeaderSeen {
		t.Fatalf("Authorization header was sent on a public request; expected it to be omitted")
	}
}

func TestClientPassesQueryParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if err := json.NewEncoder(w).Encode(map[string]any{"products": []any{}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "test", false, srv.URL)
	params := url.Values{
		"query":     {"design"},
		"tags":      {"font"},
		"min_price": {"5"},
		"max_price": {"50"},
		"sort":      {"price_asc"},
	}
	if _, err := client.Get("/products/search.json", params); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	for _, key := range []string{"query", "tags", "min_price", "max_price", "sort"} {
		if gotQuery.Get(key) == "" {
			t.Errorf("query param %q was not forwarded; got %v", key, gotQuery)
		}
	}
}

func TestClientUsesEnvBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewEncoder(w).Encode(map[string]any{"products": []any{}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvAPIBaseURL, srv.URL+"/")

	client := NewClientWithContext(t.Context(), "test", false)
	if _, err := client.Get("/products/search.json", nil); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if gotPath != "/products/search.json" {
		t.Fatalf("got path %q, want /products/search.json", gotPath)
	}
}

func TestRewritesNotFoundHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Not found"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "test", false, srv.URL)
	_, err := client.Get("/products/search.json", nil)
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if apiErr.GetHint() == "" {
		t.Fatalf("expected a hint on 404, got empty")
	}
}

func TestRewritesRateLimitHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Slow down"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "test", false, srv.URL)
	_, err := client.Get("/products/search.json", nil)
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if apiErr.GetHint() == "" {
		t.Fatalf("expected a hint on 429, got empty")
	}
}

func TestDefaultBaseURLIsProductionWhenEnvUnset(t *testing.T) {
	t.Setenv(EnvAPIBaseURL, "")
	if got := defaultBaseURL(); got != defaultAPIBaseURL {
		t.Fatalf("got %q, want %q", got, defaultAPIBaseURL)
	}
}
