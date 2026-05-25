package users

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestRefundBalanceRequiresUserID(t *testing.T) {
	cmd := newRefundBalanceCmd()
	cmd.SetArgs([]string{"--expected-email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("expected missing user ID error, got %v", err)
	}
}

func TestRefundBalanceRequiresExpectedEmail(t *testing.T) {
	cmd := newRefundBalanceCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --expected-email") {
		t.Fatalf("expected missing expected email error, got %v", err)
	}
}

func TestRefundBalanceRequiresConfirmationAfterPreview(t *testing.T) {
	var gotPost bool

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotPost = true
			t.Error("must not POST without confirmation")
		}
		testutil.JSON(t, w, map[string]any{
			"user_id":            "2245593582708",
			"count":              1,
			"total":              1334,
			"total_amount_cents": 1334,
			"currency":           "usd",
		})
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
	if gotPost {
		t.Fatal("unexpected POST")
	}
}

func TestRefundBalanceSendsPreviewedCountAndTotal(t *testing.T) {
	var requests []string
	var gotPreviewQuery, gotPostQuery, gotAuth string
	var body refundBalanceRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		gotAuth = r.Header.Get("Authorization")

		switch r.Method + " " + r.URL.Path {
		case "GET /internal/admin/users/unpaid_balance":
			gotPreviewQuery = r.URL.RawQuery
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"count":              2,
				"total":              4200,
				"total_amount_cents": 4200,
				"currency":           "usd",
			})
		case "POST /internal/admin/users/refund_balance":
			gotPostQuery = r.URL.RawQuery
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"status":             "queued",
				"message":            "Refund balance queued",
				"count":              2,
				"total":              4200,
				"total_amount_cents": 4200,
				"currency":           "usd",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Join(requests, ",") != "GET /internal/admin/users/unpaid_balance,POST /internal/admin/users/refund_balance" {
		t.Fatalf("unexpected request sequence: %v", requests)
	}
	if gotPreviewQuery != "user_id=2245593582708" {
		t.Fatalf("unexpected preview query: %q", gotPreviewQuery)
	}
	if gotPostQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotPostQuery)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || body.ExpectedPurchaseCount != 2 || body.ExpectedTotalAmountCents != 4200 {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{
		"Refund balance queued",
		"User ID: 2245593582708",
		"Status: queued",
		"Purchases: 2",
		"Amount: 4200 USD cents",
		"Currency: usd",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRefundBalanceSkipsZeroBalanceWithoutConfirmationOrPost(t *testing.T) {
	var requests []string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			t.Error("must not POST when preview has no unpaid purchases")
		}
		testutil.JSON(t, w, map[string]any{
			"user_id":            "2245593582708",
			"count":              0,
			"total":              0,
			"total_amount_cents": 0,
			"currency":           "usd",
		})
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.NoInput(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Join(requests, ",") != "GET /internal/admin/users/unpaid_balance" {
		t.Fatalf("unexpected request sequence: %v", requests)
	}
	for _, want := range []string{
		"No unpaid purchases to refund",
		"User ID: 2245593582708",
		"Status: skipped",
		"Purchases: 0",
		"Amount: 0 USD cents",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRefundBalanceSkipsZeroBalanceWithJSONOutput(t *testing.T) {
	var requests []string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			t.Error("must not POST when preview has no unpaid purchases")
		}
		testutil.JSON(t, w, map[string]any{
			"user_id":            "2245593582708",
			"count":              0,
			"total":              0,
			"total_amount_cents": 0,
			"currency":           "usd",
		})
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.NoInput(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Join(requests, ",") != "GET /internal/admin/users/unpaid_balance" {
		t.Fatalf("unexpected request sequence: %v", requests)
	}

	var resp refundBalanceResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || resp.Status != "skipped" || resp.Message != "No unpaid purchases to refund" || resp.Count != 0 || resp.TotalAmountCents != 0 || resp.Currency != "usd" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundBalanceDryRunSkipsZeroBalanceWithJSONOutput(t *testing.T) {
	var requests []string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			t.Error("dry-run must not POST when preview has no unpaid purchases")
		}
		testutil.JSON(t, w, map[string]any{
			"user_id":            "2245593582708",
			"count":              0,
			"total":              0,
			"total_amount_cents": 0,
			"currency":           "usd",
		})
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.DryRun(true), testutil.NoInput(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.Join(requests, ",") != "GET /internal/admin/users/unpaid_balance" {
		t.Fatalf("unexpected request sequence: %v", requests)
	}

	var resp refundBalanceResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || resp.Status != "skipped" || resp.Message != "No unpaid purchases to refund" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundBalanceDryRunPreviewsOnly(t *testing.T) {
	var gotPost bool

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotPost = true
			t.Error("dry-run must not POST to refund_balance")
		}
		if r.Method != http.MethodGet || r.URL.Path != "/internal/admin/users/unpaid_balance" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{
			"user_id":            "2245593582708",
			"count":              3,
			"total":              9900,
			"total_amount_cents": 9900,
			"currency":           "usd",
		})
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.DryRun(true), testutil.NoInput(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPost {
		t.Fatal("unexpected POST")
	}
	for _, want := range []string{
		"Refund balance preview",
		"User ID: 2245593582708",
		"Purchases: 3",
		"Amount: 9900 USD cents",
		"POST",
		"/internal/admin/users/refund_balance",
		"expected_email: seller@example.com",
		"expected_purchase_count: 3",
		"expected_total_amount_cents: 9900",
		"user_id: 2245593582708",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q: %q", want, out)
		}
	}
}

func TestRefundBalanceDryRunRequiresStoredMutationAuthWithEnvToken(t *testing.T) {
	called := false
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	if err := adminconfig.Delete(); err != nil {
		t.Fatalf("delete admin config: %v", err)
	}
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected stored-token policy error")
	}
	if !strings.Contains(err.Error(), "mutating admin commands require stored admin auth") ||
		!strings.Contains(err.Error(), "--non-interactive") ||
		!strings.Contains(err.Error(), adminconfig.EnvAccessToken) {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("request should not be sent")
	}
}

func TestRefundBalanceJSONPreservesPostResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /internal/admin/users/unpaid_balance":
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"count":              1,
				"total":              1334,
				"total_amount_cents": 1334,
				"currency":           "usd",
			})
		case "POST /internal/admin/users/refund_balance":
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"status":             "queued",
				"message":            "Refund balance queued",
				"count":              1,
				"total":              1334,
				"total_amount_cents": 1334,
				"currency":           "usd",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp refundBalanceResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || resp.Status != "queued" || resp.Count != 1 || resp.TotalAmountCents != 1334 {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestRefundBalancePlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /internal/admin/users/unpaid_balance":
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"count":              1,
				"total":              1334,
				"total_amount_cents": 1334,
				"currency":           "usd",
			})
		case "POST /internal/admin/users/refund_balance":
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"status":             "queued",
				"message":            "Refund balance queued",
				"count":              1,
				"total":              1334,
				"total_amount_cents": 1334,
				"currency":           "usd",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tRefund balance queued\t2245593582708\tqueued\t1\t1334\tusd"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestRefundBalanceServerErrorSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /internal/admin/users/unpaid_balance":
			testutil.JSON(t, w, map[string]any{
				"user_id":            "2245593582708",
				"count":              1,
				"total":              1334,
				"total_amount_cents": 1334,
				"currency":           "usd",
			})
		case "POST /internal/admin/users/refund_balance":
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": "Expected unpaid balance does not match current unpaid balance",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cmd := testutil.Command(newRefundBalanceCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--expected-email", "seller@example.com"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Expected unpaid balance does not match current unpaid balance") {
		t.Fatalf("expected server message in error, got %v", err)
	}
}
