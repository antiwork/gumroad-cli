package purchases

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUnblockBuyer_RequiresEmail(t *testing.T) {
	cmd := newUnblockBuyerCmd()
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnblockBuyer_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newUnblockBuyerCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestUnblockBuyer_SendsEmailOnly(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body unblockBuyerRequest

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
			"message": "Successfully unblocked buyer for purchase number 123",
		})
	})

	cmd := testutil.Command(newUnblockBuyerCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/unblock_buyer" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/unblock_buyer", gotMethod, gotPath)
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
	if !strings.Contains(out, "Successfully unblocked buyer for purchase number 123") {
		t.Errorf("expected success message in output: %q", out)
	}
}

func TestUnblockBuyer_NotBlockedShortCircuit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "not_blocked",
			"message": "Buyer is not blocked",
		})
	})

	cmd := testutil.Command(newUnblockBuyerCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Buyer is not blocked", "Status: not_blocked"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestUnblockBuyer_DryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the unblock_buyer endpoint")
	})

	cmd := testutil.Command(newUnblockBuyerCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/unblock_buyer") {
		t.Errorf("expected dry-run preview to mention POST and the unblock_buyer path, got: %q", out)
	}
	if !strings.Contains(out, "email: buyer@example.com") {
		t.Errorf("expected dry-run preview to include email, got: %q", out)
	}
}

func TestUnblockBuyer_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "not_blocked",
			"message": "Buyer is not blocked",
		})
	})

	cmd := testutil.Command(newUnblockBuyerCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp purchaseActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "not_blocked" || resp.Message != "Buyer is not blocked" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestUnblockBuyer_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "not_blocked",
			"message": "Buyer is not blocked",
		})
	})

	cmd := testutil.Command(newUnblockBuyerCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tBuyer is not blocked\t123\tnot_blocked"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
