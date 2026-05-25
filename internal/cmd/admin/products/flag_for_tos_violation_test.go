package products

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestFlagForTOSViolationRequiresProductID(t *testing.T) {
	cmd := newFlagForTOSViolationCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing product id error")
	}
	if !strings.Contains(err.Error(), "missing required argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlagForTOSViolationRequiresUserID(t *testing.T) {
	cmd := newFlagForTOSViolationCmd()
	cmd.SetArgs([]string{"abc123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing user id error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlagForTOSViolationRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not reach API without confirmation")
	})

	cmd := testutil.Command(newFlagForTOSViolationCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"abc123", "--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestFlagForTOSViolationConfirmationMessageDescribesProductFlag(t *testing.T) {
	got := fmt.Sprintf(flagForTOSViolationConfirmationMessage, "abc123", "2245593582708")
	for _, want := range []string{
		"Flag product abc123 for a policy violation on user_id 2245593582708?",
		"leaves the rest of the account online",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("confirmation message missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "suspend") {
		t.Fatalf("confirmation message must not describe a suspension: %q", got)
	}
}

func TestFlagForTOSViolationSendsUserIDProductIDAndExpectedEmail(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var body flagForTOSViolationRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"success":    true,
			"user_id":    "2245593582708",
			"product_id": "abc123",
			"status":     "flagged_for_tos_violation",
			"message":    "User flagged for a policy violation",
		})
	})

	cmd := testutil.Command(newFlagForTOSViolationCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"abc123", "--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/flag_for_tos_violation" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/flag_for_tos_violation", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.ProductID != "abc123" || body.ExpectedEmail != "seller@example.com" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{"User flagged for a policy violation", "User ID: 2245593582708", "Product ID: abc123", "Status: flagged_for_tos_violation"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestFlagForTOSViolationDryRunDoesNotContactEndpoint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the flag_for_tos_violation endpoint")
	})

	cmd := testutil.Command(newFlagForTOSViolationCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"abc123", "--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/users/flag_for_tos_violation") {
		t.Errorf("expected dry-run preview to mention POST and the flag_for_tos_violation path, got: %q", out)
	}
	for _, want := range []string{"user_id: 2245593582708", "product_id: abc123", "expected_email: seller@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected dry-run preview to include %q, got: %q", want, out)
		}
	}
}

func TestFlagForTOSViolationJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":    true,
			"user_id":    "2245593582708",
			"product_id": "abc123",
			"status":     "already_flagged",
			"message":    "User is already flagged for a policy violation",
		})
	})

	cmd := testutil.Command(newFlagForTOSViolationCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"abc123", "--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp flagForTOSViolationResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || resp.ProductID != "abc123" || resp.Status != "already_flagged" || resp.Message != "User is already flagged for a policy violation" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestFlagForTOSViolationPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"success":    true,
			"user_id":    "2245593582708",
			"product_id": "abc123",
			"status":     "flagged_for_tos_violation",
			"message":    "User flagged for a policy violation",
		})
	})

	cmd := testutil.Command(newFlagForTOSViolationCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"abc123", "--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tUser flagged for a policy violation\t2245593582708\tabc123\tflagged_for_tos_violation"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestFlagForTOSViolationProductMismatchSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Product does not belong to user",
		})
	})

	cmd := testutil.Command(newFlagForTOSViolationCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"abc123", "--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Product does not belong to user") {
		t.Fatalf("expected product mismatch error to surface, got: %v", err)
	}
}
