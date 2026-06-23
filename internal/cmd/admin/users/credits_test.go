package users

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestCreditsCommandWiresAddAndListSubcommands(t *testing.T) {
	cmd := newCreditsCmd()
	got := map[string]bool{}
	for _, sub := range cmd.Commands() {
		got[sub.Use] = true
	}

	for _, name := range []string{"add", "list"} {
		if !got[name] {
			t.Fatalf("missing credits subcommand %q in %v", name, got)
		}
	}
	if got["credit"] {
		t.Fatal("credits command must not expose confusing singular credit subcommand")
	}
}

func TestCreditsAddRequiresUserID(t *testing.T) {
	cmd := newCreditsAddCmd()
	cmd.SetArgs([]string{"--amount-cents", "1000", "--reason", "Goodwill"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("expected missing user ID error, got %v", err)
	}
}

func TestCreditsAddRequiresAmountCents(t *testing.T) {
	cmd := newCreditsAddCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--reason", "Goodwill"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --amount-cents") {
		t.Fatalf("expected missing amount error, got %v", err)
	}
}

func TestCreditsAddRequiresReason(t *testing.T) {
	cmd := newCreditsAddCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "1000"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --reason") {
		t.Fatalf("expected missing reason error, got %v", err)
	}
}

func TestCreditsAddRejectsBlankReason(t *testing.T) {
	cmd := newCreditsAddCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "1000", "--reason", "   "})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--reason cannot be empty") {
		t.Fatalf("expected blank reason error, got %v", err)
	}
}

func TestCreditsAddRejectsNonPositiveAmounts(t *testing.T) {
	for _, amount := range []string{"0", "-1"} {
		t.Run(amount, func(t *testing.T) {
			cmd := newCreditsAddCmd()
			cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents=" + amount, "--reason", "Goodwill"})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--amount-cents must be greater than 0") {
				t.Fatalf("expected positive amount error, got %v", err)
			}
		})
	}
}

func TestCreditsAddRejectsLargeAmountWithoutOverride(t *testing.T) {
	cmd := newCreditsAddCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "100001", "--reason", "Goodwill"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "$1,000 per-call cap") || !strings.Contains(err.Error(), "--allow-large-amount") {
		t.Fatalf("expected cap error, got %v", err)
	}
}

func TestCreditsAddRequiresConfirmationBeforePost(t *testing.T) {
	var stderr bytes.Buffer
	var gotPost bool
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotPost = true
		t.Error("must not POST without confirmation")
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.NoInput(true), testutil.Stderr(&stderr))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "1000", "--reason", "Goodwill"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
	if gotPost {
		t.Fatal("unexpected POST")
	}
	if !strings.Contains(stderr.String(), "Admin actor: Test Admin (admin@example.com)") {
		t.Fatalf("expected actor banner before confirmation failure, got stderr %q", stderr.String())
	}
}

func TestCreditsAddPostsJSONBodyAndRendersCredit(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body addCreditRequest

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
			"user_id": "2245593582708",
			"credit":  creditFixture(),
		})
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--expected-email", "seller@example.com",
		"--amount-cents", "1000",
		"--reason", "  Goodwill for checkout bug  ",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/users/add_credit" {
		t.Fatalf("got %s %s, want POST /internal/admin/users/add_credit", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("body fields must not appear in query string, got %q", gotQuery)
	}
	if body.UserID != "2245593582708" || body.ExpectedEmail != "seller@example.com" || body.AmountCents != 1000 || body.Reason != "Goodwill for checkout bug" {
		t.Fatalf("unexpected request body: %#v", body)
	}
	for _, want := range []string{
		"Credit issued",
		"User ID: 2245593582708",
		"Credit ID: credit_123",
		"Amount: $10.00 (1000 cents)",
		"Reason: Goodwill for checkout bug",
		"Crediting user: admin_123",
		"Created: 2026-06-03T19:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestCreditsAddAllowsLargeAmountWithOverride(t *testing.T) {
	var body addCreditRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"credit": map[string]any{
				"id":                "credit_large",
				"amount_cents":      150000,
				"reason":            "Approved large goodwill credit",
				"crediting_user_id": "admin_123",
				"created_at":        "2026-06-03T19:00:00Z",
			},
		})
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--amount-cents", "150000",
		"--reason", "Approved large goodwill credit",
		"--allow-large-amount",
	})
	testutil.MustExecute(t, cmd)

	if body.AmountCents != 150000 {
		t.Fatalf("got amount_cents %d, want 150000", body.AmountCents)
	}
}

func TestCreditsAddDryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--expected-email", "seller@example.com",
		"--amount-cents", "1000",
		"--reason", "Goodwill for checkout bug",
	})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"POST",
		"/internal/admin/users/add_credit",
		"amount_cents: 1000",
		"expected_email: seller@example.com",
		"reason: Goodwill for checkout bug",
		"user_id: 2245593582708",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %q", want, out)
		}
	}
}

func TestCreditsAddJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"credit":  creditFixture(),
		})
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "1000", "--reason", "Goodwill"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp addCreditResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || resp.Credit.ID != "credit_123" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestCreditsAddPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"credit":  creditFixture(),
		})
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "1000", "--reason", "Goodwill"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\t2245593582708\tcredit_123\t1000\tGoodwill for checkout bug\tadmin_123\t2026-06-03T19:00:00Z"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestCreditsAddConflictSurfacesServerMessageAndRetryHint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "expected_email does not match the user's current email",
		})
	})

	cmd := testutil.Command(newCreditsAddCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{
		"--user-id", "2245593582708",
		"--expected-email", "old@example.com",
		"--amount-cents", "1000",
		"--reason", "Goodwill",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected conflict error")
	}
	for _, want := range []string{
		"expected_email does not match the user's current email",
		"gumroad admin users credits list --user-id 2245593582708",
		"avoid duplicate credits",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestCreditsAddNonInteractiveUsesEnvAdminToken(t *testing.T) {
	var gotAuth string
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"credit":  creditFixture(),
		})
	})
	if err := adminconfig.Delete(); err != nil {
		t.Fatalf("delete admin config: %v", err)
	}
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")

	cmd := testutil.Command(newCreditsAddCmd(), testutil.Yes(true), testutil.NonInteractive(true))
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--amount-cents", "1000", "--reason", "Goodwill"})
	testutil.MustExecute(t, cmd)

	if gotAuth != "Bearer env-admin-token" {
		t.Fatalf("got auth %q, want Bearer env-admin-token", gotAuth)
	}
}

func TestCreditsListUsesInternalAdminEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotQuery url.Values

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		testutil.JSON(t, w, map[string]any{
			"user_id": "2245593582708",
			"credits": []map[string]any{
				creditFixture(),
				{
					"id":                "credit_zero",
					"amount_cents":      0,
					"reason":            nil,
					"crediting_user_id": nil,
					"created_at":        "2026-06-02T19:00:00Z",
				},
			},
			"pagination": map[string]any{"next": "cur-next", "limit": 50},
		})
	})

	cmd := testutil.Command(newCreditsListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--email", "seller@example.com", "--limit", "50", "--cursor", "cur-1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/users/credits" {
		t.Fatalf("got %s %s, want GET /internal/admin/users/credits", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for key, want := range map[string]string{
		"email":  "seller@example.com",
		"limit":  "50",
		"cursor": "cur-1",
	} {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q; full query %s", key, got, want, gotQuery.Encode())
		}
	}
	for _, want := range []string{
		"2 credit(s) for seller@example.com",
		"User ID: 2245593582708",
		"credit_123",
		"$10.00 (1000 cents)",
		"Goodwill for checkout bug",
		"admin_123",
		"credit_zero",
		"$0.00 (0 cents)",
		"More results: --cursor cur-next",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestCreditsListPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"credits": []map[string]any{
				creditFixture(),
				{
					"id":                "credit_456",
					"amount_cents":      -500,
					"reason":            "Historical clawback",
					"crediting_user_id": nil,
					"created_at":        "2026-06-02T19:00:00Z",
				},
			},
		})
	})

	cmd := testutil.Command(newCreditsListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := strings.Join([]string{
		"credit_123\t1000\tGoodwill for checkout bug\tadmin_123\t2026-06-03T19:00:00Z",
		"credit_456\t-500\tHistorical clawback\t\t2026-06-02T19:00:00Z",
	}, "\n")
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\ngot  %q\nwant %q", strings.TrimSpace(out), want)
	}
}

func TestCreditsListJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id":    "2245593582708",
			"credits":    []map[string]any{creditFixture()},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newCreditsListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success    bool             `json:"success"`
		UserID     string           `json:"user_id"`
		Credits    []map[string]any `json:"credits"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.UserID != "2245593582708" || len(resp.Credits) != 1 || resp.Credits[0]["id"] != "credit_123" || resp.Pagination["next"] != "cur-next" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestCreditsListEmptyResultShowsFooter(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user_id":    "2245593582708",
			"credits":    []any{},
			"pagination": map[string]any{"next": "cur-next", "limit": 20},
		})
	})

	cmd := testutil.Command(newCreditsListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--user-id", "2245593582708"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"No credits found for 2245593582708.",
		"More results: --cursor cur-next",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestCreditsListRequiresEmailOrUserID(t *testing.T) {
	cmd := newCreditsListCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "supply --email, --user-id, or --username") {
		t.Fatalf("expected missing identifier error, got %v", err)
	}
}

func TestCreditsListRejectsInvalidLimit(t *testing.T) {
	cmd := newCreditsListCmd()
	cmd.SetArgs([]string{"--user-id", "2245593582708", "--limit", "0"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected limit validation error, got %v", err)
	}
}

func creditFixture() map[string]any {
	return map[string]any{
		"id":                "credit_123",
		"amount_cents":      1000,
		"reason":            "Goodwill for checkout bug",
		"crediting_user_id": "admin_123",
		"created_at":        "2026-06-03T19:00:00Z",
	}
}
