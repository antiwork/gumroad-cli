package purchases

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestReassign_RequiresFromAndTo(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing-from", []string{"--to", "new@example.com"}, "missing required flag: --from"},
		{"missing-to", []string{"--from", "old@example.com"}, "missing required flag: --to"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newReassignCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestReassign_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newReassignCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--from", "old@example.com", "--to", "new@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestReassign_PostsBothEmails(t *testing.T) {
	var body reassignRequest
	var gotMethod, gotPath, gotQuery string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message":                 "Successfully reassigned 4 purchases from old@example.com to new@example.com. Receipt sent to new@example.com.",
			"count":                   4,
			"reassigned_purchase_ids": []string{"1", "2", "3", "4"},
		})
	})

	cmd := testutil.Command(newReassignCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--from", "old@example.com", "--to", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/reassign" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/reassign", gotMethod, gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("emails must not be sent in query string, got %q", gotQuery)
	}
	if body.From != "old@example.com" || body.To != "new@example.com" {
		t.Errorf("got from=%q to=%q, want old@example.com / new@example.com", body.From, body.To)
	}
	if !strings.Contains(out, "Reassigned: 4 purchase(s)") {
		t.Errorf("expected count line in output: %q", out)
	}
	if !strings.Contains(out, "Purchase IDs: 1, 2, 3, 4") {
		t.Errorf("expected comma-separated purchase IDs (matches repo convention via strings.Join), got: %q", out)
	}
	if strings.Contains(out, "Purchase IDs: [1 2 3 4]") {
		t.Errorf("purchase IDs must not render as Go slice debug syntax, got: %q", out)
	}
}

func TestReassign_DryRunDoesNotPost(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST")
	})

	cmd := testutil.Command(newReassignCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"--from", "old@example.com", "--to", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/reassign") {
		t.Errorf("expected dry-run preview, got: %q", out)
	}
	for _, want := range []string{"from: old@example.com", "to: new@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in dry-run preview, got: %q", want, out)
		}
	}
}

func TestReassign_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                 "Successfully reassigned 1 purchases from old@example.com to new@example.com. Receipt sent to new@example.com.",
			"count":                   1,
			"reassigned_purchase_ids": []string{"7"},
		})
	})

	cmd := testutil.Command(newReassignCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--from", "old@example.com", "--to", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool     `json:"success"`
		Count   int      `json:"count"`
		IDs     []string `json:"reassigned_purchase_ids"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Count != 1 || len(resp.IDs) != 1 || resp.IDs[0] != "7" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestReassign_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message":                 "Successfully reassigned 2 purchases from old@example.com to new@example.com. Receipt sent to new@example.com.",
			"count":                   2,
			"reassigned_purchase_ids": []string{"7", "8"},
		})
	})

	cmd := testutil.Command(newReassignCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"--from", "old@example.com", "--to", "new@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully reassigned 2 purchases from old@example.com to new@example.com. Receipt sent to new@example.com.\told@example.com\tnew@example.com\t2"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestReassign_JSONIncludesVerifyStateHint(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "No purchases found for email: old@example.com",
		})
	})

	cmd := testutil.Command(newReassignCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--from", "old@example.com", "--to", "new@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected reassign error to surface")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected wrap to keep an *api.APIError on the chain, got %T: %v", err, err)
	}
	if !strings.Contains(apiErr.Error(), "reassign request failed:") {
		t.Errorf("APIError.Message must carry the wrap prefix: %q", apiErr.Error())
	}
	if !strings.Contains(apiErr.Error(), "gumroad admin purchases search --email old@example.com") {
		t.Errorf("APIError.Message must carry verify-state guidance pointing at search: %q", apiErr.Error())
	}
}
