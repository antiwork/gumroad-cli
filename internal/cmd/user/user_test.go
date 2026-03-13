package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestUser_JSON(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":        "Test User",
				"email":       "test@example.com",
				"bio":         "A test bio",
				"profile_url": "https://gumroad.com/test",
			},
		})
	})

	cmd := testutil.Command(NewUserCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })

	if gotPath != "/user" {
		t.Errorf("got path %q, want /user", gotPath)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	user := resp["user"].(map[string]any)
	if user["email"] != "test@example.com" {
		t.Errorf("got email=%v, want test@example.com", user["email"])
	}
}

func TestUser_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":        "Test User",
				"email":       "test@example.com",
				"profile_url": "https://gumroad.com/test",
			},
		})
	})

	cmd := testutil.Command(NewUserCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })

	if !strings.Contains(out, "Test User\t") {
		t.Errorf("plain output missing tab-separated name: %q", out)
	}
	if !strings.Contains(out, "test@example.com") {
		t.Errorf("plain output missing email: %q", out)
	}
}

func TestUser_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":        "Jane Doe",
				"email":       "jane@example.com",
				"bio":         "Creator of things",
				"profile_url": "https://gumroad.com/jane",
			},
		})
	})

	cmd := NewUserCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })

	if !strings.Contains(out, "Jane Doe") {
		t.Errorf("output missing name: %q", out)
	}
	if !strings.Contains(out, "jane@example.com") {
		t.Errorf("output missing email: %q", out)
	}
	if !strings.Contains(out, "Creator of things") {
		t.Errorf("output missing bio: %q", out)
	}
	if !strings.Contains(out, "https://gumroad.com/jane") {
		t.Errorf("output missing profile URL: %q", out)
	}
}

func TestUser_NoBio(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":  "No Bio",
				"email": "nobio@example.com",
			},
		})
	})

	cmd := NewUserCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })

	if !strings.Contains(out, "No Bio") {
		t.Errorf("output missing name: %q", out)
	}
}

func TestUser_RawFixture(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/user_raw.json"))
	})

	cmd := NewUserCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })

	if !strings.Contains(out, "Raw User") || !strings.Contains(out, "raw@example.com") {
		t.Errorf("output missing raw fixture user data: %q", out)
	}
}

func TestUser_JQ(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":  "JQ User",
				"email": "jq@example.com",
			},
		})
	})

	cmd := testutil.Command(NewUserCmd(), testutil.JQ(".user.email"))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })

	if strings.TrimSpace(out) != `"jq@example.com"` {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), `"jq@example.com"`)
	}
}

func TestUser_DebugWritesToStderrOnly(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/user_raw.json"))
	})

	cmd := testutil.Command(NewUserCmd(), testutil.Debug(true))
	stdout, stderr := testutil.CaptureOutput(func() { _ = cmd.RunE(cmd, []string{}) })

	if !strings.Contains(stdout, "Raw User") || strings.Contains(stdout, "DEBUG") {
		t.Errorf("stdout should contain only user output, got: %q", stdout)
	}
	if !strings.Contains(stderr, "DEBUG request method=GET") || !strings.Contains(stderr, "status=200") {
		t.Errorf("stderr should contain debug output, got: %q", stderr)
	}
}

func TestUser_DebugHonorsConfiguredStderr(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/user_raw.json"))
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := testutil.Command(NewUserCmd(), testutil.Debug(true), testutil.Stdout(&stdout), testutil.Stderr(&stderr))
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "Raw User") {
		t.Fatalf("stdout missing user output: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "DEBUG request method=GET") || !strings.Contains(stderr.String(), "status=200") {
		t.Fatalf("configured stderr missing debug output: %q", stderr.String())
	}
}

func TestUser_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Unauthorized"}); err != nil {
			t.Fatalf("encode unauthorized response: %v", err)
		}
	})

	cmd := NewUserCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestUser_NotAuthenticated(t *testing.T) {
	// Empty config — no token
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without token")
	})

	// Overwrite config with empty token
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	// No config file at all

	cmd := NewUserCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error without auth")
	}
	if !strings.Contains(err.Error(), "gumroad auth login") {
		t.Errorf("error should mention gumroad auth login: %v", err)
	}
}

func TestUser_InvalidResponse(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"user":`)
	})

	cmd := NewUserCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestUser_WriteError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":        "Write Fail",
				"email":       "write@example.com",
				"profile_url": "https://gumroad.com/write",
			},
		})
	})

	cmd := testutil.Command(NewUserCmd(), testutil.Stdout(failingWriter{}))
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got: %v", err)
	}
}
