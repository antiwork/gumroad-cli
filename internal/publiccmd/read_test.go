package publiccmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/publicapi"
)

type sampleResponse struct {
	Total    int      `json:"total"`
	Products []string `json:"products"`
}

func newServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, cmdutil.Options) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv(publicapi.EnvAPIBaseURL, srv.URL)

	opts := cmdutil.DefaultOptions()
	opts.Quiet = true
	opts.Version = "test"
	return srv, opts
}

func TestRunGetDecoded_DecodesBody(t *testing.T) {
	_, opts := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"total": 2, "products": []string{"a", "b"}}); err != nil {
			t.Fatalf("encode: %v", err)
		}
	})

	var got sampleResponse
	err := RunGetDecoded[sampleResponse](opts, "fetching...", "/products/search.json", url.Values{}, func(resp sampleResponse) error {
		got = resp
		return nil
	})
	if err != nil {
		t.Fatalf("RunGetDecoded: %v", err)
	}
	if got.Total != 2 || len(got.Products) != 2 {
		t.Fatalf("decoded payload: %+v", got)
	}
}

func TestRunGetDecoded_ForwardsParams(t *testing.T) {
	var seenQuery url.Values
	_, opts := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"total":0,"products":[]}`))
	})

	err := RunGetDecoded[sampleResponse](opts, "", "/products/search.json", url.Values{"query": {"hello"}}, func(sampleResponse) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunGetDecoded: %v", err)
	}
	if seenQuery.Get("query") != "hello" {
		t.Fatalf("query not forwarded: %v", seenQuery)
	}
}

func TestRunGetDecoded_PropagatesAPIError(t *testing.T) {
	_, opts := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"message":"boom"}`))
	})

	err := RunGetDecoded[sampleResponse](opts, "", "/products/search.json", nil, func(sampleResponse) error {
		t.Fatal("render should not run on error")
		return nil
	})
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
}

func TestNewAPIClient_NotNil(t *testing.T) {
	opts := cmdutil.DefaultOptions()
	opts.Version = "test"
	client := NewAPIClient(opts)
	if client == nil {
		t.Fatal("NewAPIClient returned nil")
	}
}
