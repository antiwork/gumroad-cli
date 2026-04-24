package adminapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/api"
)

func TestClientPrefixesInternalAdminPath(t *testing.T) {
	var gotPath string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "admin-token", "test", false, srv.URL)
	if _, err := client.Get("/purchases/123", nil); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if gotPath != "/internal/admin/purchases/123" {
		t.Fatalf("got path %q, want /internal/admin/purchases/123", gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
}

func TestClientUsesAdminBaseURLEnv(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvAPIBaseURL, srv.URL+"/")

	client := NewClientWithContext(t.Context(), "tok", "test", false)
	if _, err := client.Get("/users/suspension", nil); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if gotPath != "/internal/admin/users/suspension" {
		t.Fatalf("got path %q, want /internal/admin/users/suspension", gotPath)
	}
}

func TestClientRedactsQueryValuesInDebug(t *testing.T) {
	var debug strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "admin-token", "test", true, srv.URL)
	client.SetDebugWriter(&debug)

	params := url.Values{"license_key": {"SECRET-LICENSE"}}
	if _, err := client.Get("/licenses/lookup", params); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if strings.Contains(debug.String(), "SECRET-LICENSE") {
		t.Fatalf("debug output leaked query value: %q", debug.String())
	}
	if !strings.Contains(debug.String(), "license_key=REDACTED") {
		t.Fatalf("debug output should show redacted query, got %q", debug.String())
	}
}

func TestClientRewritesAdminAuthHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Nope"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithBaseURL(t.Context(), "bad-token", "test", false, srv.URL)
	_, err := client.Get("/purchases/123", nil)
	if err == nil {
		t.Fatal("expected auth error")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.GetHint() != adminconfig.HintSetAdminToken {
		t.Fatalf("got hint %q, want %q", apiErr.GetHint(), adminconfig.HintSetAdminToken)
	}
}
