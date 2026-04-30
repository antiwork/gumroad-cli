package purchases

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestResendReceipt_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newResendReceiptCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestResendReceipt_PostsToCorrectPath(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully resent receipt for purchase number 123 to buyer@example.com",
		})
	})

	cmd := testutil.Command(newResendReceiptCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/resend_receipt" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/resend_receipt", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if !strings.Contains(out, "Successfully resent receipt for purchase number 123") {
		t.Errorf("expected success message in output: %q", out)
	}
}

func TestResendReceipt_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newResendReceiptCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/resend_receipt") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
}

func TestResendReceipt_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully resent receipt for purchase number 123 to buyer@example.com",
		})
	})

	cmd := testutil.Command(newResendReceiptCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || !strings.Contains(resp.Message, "purchase number 123") {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestResendReceipt_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully resent receipt for purchase number 123 to buyer@example.com",
		})
	})

	cmd := testutil.Command(newResendReceiptCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully resent receipt for purchase number 123 to buyer@example.com\t123"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
