package admincmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestFetchPostJSONInteractiveRequiresStoredTokenWhenEnvIsSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(srv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, srv.URL)

	_, err := FetchPostJSON(testutil.TestOptions(), "Posting...", "/users/reset_password", struct{}{})
	if err == nil {
		t.Fatal("expected stored-token policy error")
	}
	if !strings.Contains(err.Error(), "--non-interactive") ||
		!strings.Contains(err.Error(), "gumroad auth login") ||
		!strings.Contains(err.Error(), adminconfig.EnvAccessToken) {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("request should not be sent")
	}
}

func TestFetchPostJSONNonInteractiveUsesEnvToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{"success": true})
	}))
	t.Cleanup(srv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, srv.URL)
	var errOut bytes.Buffer

	_, err := FetchPostJSON(testutil.TestOptions(testutil.NonInteractive(true), testutil.Stderr(&errOut)), "Posting...", "/users/reset_password", struct{}{})
	if err != nil {
		t.Fatalf("FetchPostJSON failed: %v", err)
	}
	if gotAuth != "Bearer env-admin-token" {
		t.Fatalf("got Authorization=%q, want Bearer env-admin-token", gotAuth)
	}
	if strings.Contains(errOut.String(), "Admin actor:") {
		t.Fatalf("non-interactive mode should not print actor banner, got %q", errOut.String())
	}
}

func TestFetchPostJSONShowsStoredActorBanner(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := adminconfig.Save(&adminconfig.Config{
		Token: "stored-admin-token",
		Actor: adminconfig.Actor{
			Name:  "Test Admin",
			Email: "admin@example.com",
		},
	}); err != nil {
		t.Fatalf("Save admin config failed: %v", err)
	}
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{"success": true})
	}))
	t.Cleanup(srv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, srv.URL)
	var errOut bytes.Buffer

	_, err := FetchPostJSON(testutil.TestOptions(testutil.Stderr(&errOut)), "Posting...", "/users/reset_password", struct{}{})
	if err != nil {
		t.Fatalf("FetchPostJSON failed: %v", err)
	}
	if gotAuth != "Bearer stored-admin-token" {
		t.Fatalf("got Authorization=%q, want Bearer stored-admin-token", gotAuth)
	}
	if !strings.Contains(errOut.String(), "Admin actor: Test Admin (admin@example.com)") {
		t.Fatalf("expected actor banner, got %q", errOut.String())
	}
}

func TestRunPostJSONDecodedAllowsEnvTokenForReadEndpoints(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{"success": true})
	}))
	t.Cleanup(srv.Close)
	t.Setenv(adminapi.EnvAPIBaseURL, srv.URL)

	err := RunPostJSONDecoded[map[string]any](testutil.TestOptions(), "Fetching...", "/purchases/search", struct{}{}, func(resp map[string]any) error {
		if resp["success"] != true {
			t.Fatalf("unexpected response: %#v", resp)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunPostJSONDecoded failed: %v", err)
	}
	if gotAuth != "Bearer env-admin-token" {
		t.Fatalf("got Authorization=%q, want Bearer env-admin-token", gotAuth)
	}
}

func TestFetchMutationDoesNotRunRequestWhenTokenPolicyFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	ran := false

	_, err := FetchMutation(testutil.TestOptions(), "Posting...", func(*adminapi.Client) (json.RawMessage, error) {
		ran = true
		return nil, nil
	})
	if !errors.Is(err, adminconfig.ErrNotAuthenticated) {
		t.Fatalf("expected stored-token policy error, got %v", err)
	}
	if ran {
		t.Fatal("request runner must not be called when the token policy fails")
	}
}

func TestFetchMutationReturnsRunnerResultAndError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")
	opts := testutil.TestOptions(testutil.NonInteractive(true))

	data, err := FetchMutation(opts, "Posting...", func(*adminapi.Client) (json.RawMessage, error) {
		return json.RawMessage(`{"success":true}`), nil
	})
	if err != nil || string(data) != `{"success":true}` {
		t.Fatalf("got data=%q err=%v", data, err)
	}

	runnerErr := errors.New("request outcome unknown")
	_, err = FetchMutation(opts, "Posting...", func(*adminapi.Client) (json.RawMessage, error) {
		return nil, runnerErr
	})
	if !errors.Is(err, runnerErr) {
		t.Fatalf("expected runner error to pass through, got %v", err)
	}
}
