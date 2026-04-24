package affiliates

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/affiliates" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"affiliates": []map[string]any{
				{"id": "a1", "email": "partner1@example.com", "commission_percentage": 20},
				{"id": "a2", "email": "partner2@example.com", "commission_percentage": 50},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &resp); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jsonErr, out)
	}
	affiliates := resp["affiliates"].([]any)
	if len(affiliates) != 2 {
		t.Errorf("got %d affiliates, want 2", len(affiliates))
	}
}

func TestList_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"affiliates": []map[string]any{
				{"id": "a1", "email": "partner@example.com", "commission_percentage": 20},
			},
		})
	})

	cmd := newListCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
	if !strings.Contains(out, "a1") || !strings.Contains(out, "partner@example.com") || !strings.Contains(out, "20%") {
		t.Errorf("table output missing affiliate data: %q", out)
	}
}

func TestCreate_Success(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "new@example.com", "--commission", "30"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotPath != "/affiliates" {
		t.Errorf("got path %q, want /affiliates", gotPath)
	}
	if !strings.Contains(out, "created successfully") {
		t.Errorf("expected success message, got: %q", out)
	}
}

func TestCreate_MissingEmail(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--commission", "30"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "required flag(s) \"email\" not set") {
		t.Errorf("expected missing email error, got: %v", err)
	}
}
