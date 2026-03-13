package customfields

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"custom_fields": []map[string]any{
				{"name": "Company", "required": true, "type": "text"},
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

	if gotPath != "/products/p1/custom_fields" {
		t.Errorf("got path %q", gotPath)
	}
	if !strings.Contains(out, "Company") {
		t.Errorf("output missing field name: %q", out)
	}
}

func TestList_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
}

func TestCreate_Params(t *testing.T) {
	var gotName, gotRequired, gotType string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotName = r.PostForm.Get("name")
		gotRequired = r.PostForm.Get("required")
		gotType = r.PostForm.Get("type")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone Number", "--required", "--type", "phone"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotName != "Phone Number" {
		t.Errorf("got name=%q, want 'Phone Number'", gotName)
	}
	if gotRequired != "true" {
		t.Errorf("got required=%q, want true", gotRequired)
	}
	if gotType != "phone" {
		t.Errorf("got type=%q, want phone", gotType)
	}
}

func TestCreate_NormalizesType(t *testing.T) {
	var gotType string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotType = r.PostForm.Get("type")
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone Number", "--type", " Phone "})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotType != "phone" {
		t.Errorf("got type=%q, want phone", gotType)
	}
}

func TestCreate_RejectsInvalidType(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone Number", "--type", "Phone Number"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--type must use lowercase letters, numbers, hyphens, or underscores") {
		t.Fatalf("expected type validation error, got: %v", err)
	}
}

func TestCreate_NameRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --name")
	}
}

func TestUpdate_NameEscaping(t *testing.T) {
	var gotRawPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.RawPath
		if gotRawPath == "" {
			gotRawPath = r.RequestURI
		}
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "Full Name", "--required"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(gotRawPath, "Full%20Name") {
		t.Errorf("name should be URL-escaped in path, got: %q", gotRawPath)
	}
}

func TestDelete_NameEscaping(t *testing.T) {
	var gotRawPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.RawPath
		if gotRawPath == "" {
			gotRawPath = r.RequestURI
		}
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--product", "p1", "--name", "Company & Title"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if !strings.Contains(gotRawPath, "%20") {
		t.Errorf("spaces should be URL-escaped in path, got: %q", gotRawPath)
	}
	if !strings.Contains(gotRawPath, "Company") || !strings.Contains(gotRawPath, "Title") {
		t.Errorf("name should appear in path, got: %q", gotRawPath)
	}
}

func TestDelete_RequiresConfirmation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.NoInput(true))
	cmd.SetArgs([]string{"--product", "p1", "--name", "Field"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without confirmation")
	}
}

func TestDelete_NameRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --name")
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"custom_fields": []map[string]any{
				{"name": "Company", "required": true, "type": "text"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
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

func TestList_RawFixture(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/list_raw.json"))
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})
	if !strings.Contains(out, "Company") || !strings.Contains(out, "Phone") {
		t.Errorf("raw fixture output missing custom fields: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"custom_fields": []map[string]any{
				{"name": "Company", "required": true, "type": "text"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})
	if !strings.Contains(out, "Company") {
		t.Errorf("plain output missing data: %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"custom_fields": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})
	if !strings.Contains(out, "No custom fields found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestCreate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newCreateCmd()
	cmd.SetArgs([]string{"--name", "Field"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
}

func TestCreate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"custom_field": map[string]any{"name": "Phone"}})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone"})
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

func TestUpdate_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--name", "Field", "--required"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
}

func TestUpdate_NameRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--required"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --name")
	}
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one field to update must be provided") {
		t.Fatalf("expected no-op update error, got: %v", err)
	}
}

func TestUpdate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"custom_field": map[string]any{"name": "Phone"}})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone", "--required"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestDelete_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true))
	cmd.SetArgs([]string{"--name", "Field"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
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

func TestCreate_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "created") {
		t.Errorf("expected created message, got: %q", out)
	}
}

func TestUpdate_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--name", "Phone", "--required"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "updated") {
		t.Errorf("expected updated message, got: %q", out)
	}
}

func TestDelete_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDeleteCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--name", "Field"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected deleted message, got: %q", out)
	}
}
