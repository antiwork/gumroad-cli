package webhooks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_ResourceRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without --resource")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --resource")
	}
	if !strings.Contains(err.Error(), "--resource") {
		t.Errorf("error should mention --resource: %v", err)
	}
}

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath, gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"resource_subscriptions": []map[string]any{
				{"id": "rs1", "resource_name": "sale", "post_url": "https://example.com/hook"},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--resource", "sale"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotPath != "/resource_subscriptions" {
		t.Errorf("got path %q, want /resource_subscriptions", gotPath)
	}
	if !strings.Contains(gotQuery, "resource_name=sale") {
		t.Errorf("query missing resource_name=sale: %q", gotQuery)
	}
	if !strings.Contains(out, "rs1") {
		t.Errorf("output missing subscription ID: %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"resource_subscriptions": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--resource", "refund"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "No webhooks found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestCreate_Params(t *testing.T) {
	var gotMethod, gotResource, gotURL string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotResource = r.PostForm.Get("resource_name")
		gotURL = r.PostForm.Get("post_url")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--resource", "sale", "--url", "https://example.com/hook"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotResource != "sale" {
		t.Errorf("got resource_name=%q, want sale", gotResource)
	}
	if gotURL != "https://example.com/hook" {
		t.Errorf("got post_url=%q", gotURL)
	}
}

func TestCreate_ResourceRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--url", "https://example.com"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --resource")
	}
}

func TestCreate_URLRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--resource", "sale"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --url")
	}
}

func TestCreate_InvalidURL(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API with invalid URL")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--resource", "sale", "--url", "ftp://example.com/hook"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--url must use http or https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without confirmation")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	err := cmd.RunE(cmd, []string{"rs1"})
	if err == nil {
		t.Fatal("expected error without confirmation")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
}

func TestDelete_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"rs1"}) })

	if gotMethod != "DELETE" || gotPath != "/resource_subscriptions/rs1" {
		t.Errorf("got %s %s, want DELETE /resource_subscriptions/rs1", gotMethod, gotPath)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"resource_subscriptions": []map[string]any{
				{"id": "rs1", "resource_name": "sale", "post_url": "https://example.com/hook"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--resource", "sale"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestList_RawFixture(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/list_raw.json"))
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--resource", "sale"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "rs1") || !strings.Contains(out, "https://example.com/hook") {
		t.Errorf("raw fixture output missing webhook data: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"resource_subscriptions": []map[string]any{
				{"id": "rs1", "resource_name": "sale", "post_url": "https://example.com/hook"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--resource", "sale"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "rs1") {
		t.Errorf("plain output missing data: %q", out)
	}
}

func TestList_NoColorDisablesANSI(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"resource_subscriptions": []map[string]any{
				{"id": "rs1", "resource_name": "sale", "post_url": "https://example.com/hook"},
			},
		})
	})
	testutil.SetStdoutIsTerminal(t, true)

	cmd := testutil.Command(newListCmd(), testutil.NoColor(true))
	cmd.SetArgs([]string{"--resource", "sale"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	testutil.AssertNoANSI(t, out)
}

func TestCreate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"resource_subscription": map[string]any{"id": "rs1"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--resource", "sale", "--url", "https://example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--resource", "sale"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreate_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"resource_subscription": map[string]any{
				"id":            "rs1",
				"resource_name": "sale",
				"post_url":      "https://example.com",
			},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--resource", "sale", "--url", "https://example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "Created webhook:") {
		t.Errorf("expected created message, got: %q", out)
	}
	if !strings.Contains(out, "rs1") {
		t.Errorf("expected webhook ID in output, got: %q", out)
	}
	if !strings.Contains(out, "sale") {
		t.Errorf("expected resource name in output, got: %q", out)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("expected post URL in output, got: %q", out)
	}
}

func TestCreate_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"resource_subscription": map[string]any{
				"id":            "rs1",
				"resource_name": "sale",
				"post_url":      "https://example.com",
			},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--resource", "sale", "--url", "https://example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "rs1\tsale\thttps://example.com") {
		t.Errorf("expected plain tab-separated output, got: %q", out)
	}
}

func TestCreate_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--resource", "sale", "--url", "https://example.com"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDelete_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true), testutil.Quiet(false))
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"rs1"}) })
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected deleted message, got: %q", out)
	}
}

func TestDelete_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	err := cmd.RunE(cmd, []string{"rs1"})
	if err == nil {
		t.Fatal("expected error")
	}
}
