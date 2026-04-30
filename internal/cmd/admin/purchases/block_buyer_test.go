package purchases

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestBlockBuyer_RequiresEmail(t *testing.T) {
	cmd := newBlockBuyerCmd()
	cmd.SetArgs([]string{"123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing email error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlockBuyer_RequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --yes and --no-input")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestBlockBuyer_OmitsCommentWhenAbsent(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotQuery string
	var body blockBuyerRequest
	var bodyKeys map[string]json.RawMessage

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
		if err := json.Unmarshal(raw, &bodyKeys); err != nil {
			t.Fatalf("decode body keys: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully blocked buyer for purchase number 123",
		})
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/internal/admin/purchases/123/block_buyer" {
		t.Fatalf("got %s %s, want POST /internal/admin/purchases/123/block_buyer", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	if gotQuery != "" {
		t.Fatalf("email/comment must not appear in query string, got %q", gotQuery)
	}
	if body.Email != "buyer@example.com" {
		t.Fatalf("got email %q, want buyer@example.com", body.Email)
	}
	if _, present := bodyKeys["comment_content"]; present {
		t.Errorf("comment_content must be omitted when not set, got body keys: %v", bodyKeys)
	}
	if !strings.Contains(out, "Successfully blocked buyer for purchase number 123") {
		t.Errorf("expected success message in output: %q", out)
	}
}

func TestBlockBuyer_ForwardsCommentContent(t *testing.T) {
	var body blockBuyerRequest

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully blocked buyer for purchase number 123",
		})
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--comment", "Refund abuse"})
	testutil.MustExecute(t, cmd)

	if body.CommentContent != "Refund abuse" {
		t.Errorf("expected comment_content=%q, got %#v", "Refund abuse", body)
	}
}

func TestBlockBuyer_AlreadyBlockedShortCircuit(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "already_blocked",
			"message": "Buyer is already blocked",
		})
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{"Buyer is already blocked", "Status: already_blocked"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestBlockBuyer_DryRunIncludesComment(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not POST to the block_buyer endpoint")
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.DryRun(true), testutil.NoInput(true))
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com", "--comment", "Refund abuse"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "POST") || !strings.Contains(out, "/internal/admin/purchases/123/block_buyer") {
		t.Errorf("expected dry-run preview to mention POST and the block_buyer path, got: %q", out)
	}
	if !strings.Contains(out, "email: buyer@example.com") || !strings.Contains(out, "comment_content: Refund abuse") {
		t.Errorf("expected dry-run preview to include email and comment_content, got: %q", out)
	}
}

func TestBlockBuyer_JSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "already_blocked",
			"message": "Buyer is already blocked",
		})
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.Yes(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp purchaseActionResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success || resp.Status != "already_blocked" || resp.Message != "Buyer is already blocked" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestBlockBuyer_PlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"message": "Successfully blocked buyer for purchase number 123",
		})
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tSuccessfully blocked buyer for purchase number 123\t123\t"
	if strings.TrimSpace(out) != strings.TrimSpace(want) {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestBlockBuyer_PlainOutputAlreadyBlockedIncludesStatus(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"status":  "already_blocked",
			"message": "Buyer is already blocked",
		})
	})

	cmd := testutil.Command(newBlockBuyerCmd(), testutil.Yes(true), testutil.PlainOutput())
	cmd.SetArgs([]string{"123", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "true\tBuyer is already blocked\t123\talready_blocked"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output: %q", out)
	}
}
