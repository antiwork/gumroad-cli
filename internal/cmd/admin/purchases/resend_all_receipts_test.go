package purchases

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestResendAllReceipts_RequiresEmail(t *testing.T) {
	cmd := newResendAllReceiptsCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResendAllReceipts_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newResendAllReceiptsCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestResendAllReceipts_SendsEmail(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body resendAllReceiptsRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully resent all receipts to buyer@example.com",
			"count":   3,
		})
	})

	cmd := testutil.Command(newResendAllReceiptsCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/resend_all_receipts" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/resend_all_receipts", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "buyer@example.com" {
		t.Fatalf("got email %q, want buyer@example.com", body.Email)
	}
	if !strings.Contains(out, "Purchases included: 3") {
		t.Errorf("expected count in output: %q", out)
	}
}

func TestResendAllReceipts_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newResendAllReceiptsCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/resend_all_receipts") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	if !strings.Contains(out, "email: buyer@example.com") {
		t.Errorf("expected dry-run preview to include email, got: %q", out)
	}
}

func TestResendAllReceipts_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully resent all receipts to buyer@example.com",
			"count":   2,
		})
	})

	cmd := testutil.Command(newResendAllReceiptsCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Count != 2 {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestResendAllReceipts_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully resent all receipts to buyer@example.com",
			"count":   2,
		})
	})

	cmd := testutil.Command(newResendAllReceiptsCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully resent all receipts to buyer@example.com\tbuyer@example.com\t2"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestResendAllReceipts_NotFoundSurfaces(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "No purchases found for email: buyer@example.com",
		})
	})

	cmd := testutil.Command(newResendAllReceiptsCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "No purchases found for email") {
		t.Errorf("missing underlying message: %v", err)
	}
}
