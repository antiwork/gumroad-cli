package categories

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func categoriesHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"variant_categories": []map[string]any{
				{"id": "vc1", "title": "Size"},
				{"id": "vc2", "title": "Color"},
			},
		})
	}
}

func categoryHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"variant_category": map[string]any{"id": "vc1", "title": "Size"},
		})
	}
}

// --- List ---

func TestList_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"variant_categories": []map[string]any{
				{"id": "vc1", "title": "Size"},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotPath != "/products/p1/variant_categories" {
		t.Errorf("got path %q", gotPath)
	}
	if !strings.Contains(out, "Size") {
		t.Errorf("output missing title: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, categoriesHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	cats := resp["variant_categories"].([]any)
	if len(cats) != 2 {
		t.Errorf("got %d categories, want 2", len(cats))
	}
}

func TestList_RawFixture(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/list_raw.json"))
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Size") || !strings.Contains(out, "Color") {
		t.Errorf("raw fixture output missing categories: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, categoriesHandler(t))

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "vc1") || !strings.Contains(out, "Size") {
		t.Errorf("plain missing data: %q", out)
	}
}

func TestList_NoColorDisablesANSI(t *testing.T) {
	testutil.Setup(t, categoriesHandler(t))
	testutil.SetStdoutIsTerminal(t, true)

	cmd := testutil.Command(newListCmd(), testutil.NoColor(true))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	testutil.AssertNoANSI(t, out)
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant_categories": []map[string]any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "No variant categories found") {
		t.Errorf("expected empty message: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- View ---

func TestView_Table(t *testing.T) {
	testutil.Setup(t, categoryHandler(t))

	cmd := newViewCmd()
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "Size") {
		t.Errorf("missing title: %q", out)
	}
	if !strings.Contains(out, "vc1") {
		t.Errorf("missing ID: %q", out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, categoryHandler(t))

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, categoryHandler(t))

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(out, "vc1") || !strings.Contains(out, "Size") {
		t.Errorf("plain missing data: %q", out)
	}
}

func TestView_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newViewCmd()
	cmd.SetArgs([]string{"vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

// --- Create ---

func TestCreate_SendsTitle(t *testing.T) {
	var gotTitle string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotTitle = r.PostForm.Get("title")
		testutil.JSON(t, w, map[string]any{"variant_category": map[string]any{"id": "vc1", "title": "Size"}})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--title", "Size"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotTitle != "Size" {
		t.Errorf("got title=%q, want Size", gotTitle)
	}
}

func TestCreate_TitleRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--title") {
		t.Fatalf("expected --title required, got: %v", err)
	}
}

func TestCreate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--title", "Size"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestCreate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant_category": map[string]any{"id": "vc1"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--title", "Size"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestCreate_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant_category": map[string]any{"id": "vc1", "title": "Size"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--title", "Size"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "Created variant category:") {
		t.Errorf("expected created message, got: %q", out)
	}
	if !strings.Contains(out, "vc1") {
		t.Errorf("expected category ID in output, got: %q", out)
	}
	if !strings.Contains(out, "Size") {
		t.Errorf("expected category title in output, got: %q", out)
	}
}

func TestCreate_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant_category": map[string]any{"id": "vc1", "title": "Size"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--title", "Size"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "vc1\tSize") {
		t.Errorf("expected plain tab-separated output, got: %q", out)
	}
}

func TestCreate_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		testutil.JSON(t, w, map[string]any{"message": "Invalid"})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--title", "Size"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Update ---

func TestUpdate_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotTitle string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotTitle = r.PostForm.Get("title")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"vc1", "--product", "p1", "--title", "Color"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/p1/variant_categories/vc1" {
		t.Errorf("got path %q", gotPath)
	}
	if gotTitle != "Color" {
		t.Errorf("got title=%q, want Color", gotTitle)
	}
}

func TestUpdate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one field to update must be provided") {
		t.Fatalf("expected no-op update error, got: %v", err)
	}
}

func TestUpdate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"variant_category": map[string]any{"id": "vc1"}})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"vc1", "--product", "p1", "--title", "Color"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

// --- Delete ---

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	err := cmd.Execute()
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
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "DELETE" || gotPath != "/products/p1/variant_categories/vc1" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
}

func TestDelete_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newDeleteCmd()
	cmd.SetArgs([]string{"vc1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--product") {
		t.Fatalf("expected --product required, got: %v", err)
	}
}

func TestDelete_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testutil.JSON(t, w, map[string]any{"message": "Not found"})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"vc1", "--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404")
	}
}
